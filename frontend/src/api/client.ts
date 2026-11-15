import { BUDDY_REPLY, mockHome, mockInterns, mockResolve, mockReviews, mockSubmit, mockUser } from './mock'
import { readSse } from './sse'
import type { ChatMessage, FocusTask, HomeData, InternProgress, ReviewRequest, StreamHandlers, User, UserKind } from './types'

/**
 * Единственная точка общения с бэкендом (Go API из backend/cmd/api).
 *
 * Режимы (переменная VITE_API_MODE):
 *   api  — по умолчанию: настоящие запросы к Go API. В dev Vite проксирует /api на VITE_API_URL
 *          (по умолчанию http://localhost:8080), поэтому базовый адрес пустой.
 *   mock — без сети: данные из mock.ts, ответ Buddy — заглушка.
 * VITE_API_BASE — абсолютный адрес API для сборки без прокси (например http://localhost:8080);
 * Go отдаёт CORS "*", токен идёт в заголовке, так что прямые запросы тоже работают.
 *
 * Ручки (backend/internal/httpapi/server.go):
 *   POST /api/login                       {login}            → {token, user}
 *   GET  /api/me                                              → user
 *   POST /api/conversations                {title}           → conversation
 *   GET  /api/conversations                                   → {conversations}
 *   GET  /api/conversations/{id}/messages?limit=N             → {messages}
 *   POST /api/chat  {conversation_id, client_message_id, content} → SSE: accepted, status,
 *        message_start, text_delta, message_done, warning, done | error
 *   PATCH /api/checklist/tasks/{id}        {status}          → {task, progress}
 *   GET  /api/reviews                                         → {reviews}
 *   POST /api/reviews/{task_id}/approve | /reject {comment}  → {task_id, status}
 *   GET  /api/interns                                         → {interns}  (наставник)
 * Апрув задач — docs/approvals-contract.md.
 */

const env = import.meta.env
export const API_MODE: 'api' | 'mock' = env.VITE_API_MODE === 'mock' ? 'mock' : 'api'
const BASE = String(env.VITE_API_BASE ?? '').replace(/\/$/, '')
/** Вход имитирует корпоративный SSO: тестовая учётка без пароля (см. migrations/0003_initData). */
const DEMO_LOGIN = String(env.VITE_DEMO_LOGIN || 'sasha.intern')
/** Если агент молчит дольше, запрос обрывается и показывается «Повторить».
 * Пока модель решает, вызвать ли инструмент, событий нет вовсе: на CPU с длинной историей
 * это 40 с – 2,5 мин (замерено). Таймаут ловит только мёртвое соединение, поэтому с запасом;
 * бэкенд сам считает ход зависшим через 5 минут. */
export const IDLE_TIMEOUT_MS = 180_000

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string) {
    super(code)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

// ─── токен ───
const TOKEN_KEY = 'buddy.token'
let token: string | null = readToken()
let conversationId: number | null = null
/** Растёт, когда текущий чат меняется не из загрузки истории: вход, выход, новый чат, первое сообщение. */
let conversationVersion = 0
let unauthorized: (() => void) | null = null

function readToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}
function saveToken(value: string | null) {
  token = value
  try {
    if (value) sessionStorage.setItem(TOKEN_KEY, value)
    else sessionStorage.removeItem(TOKEN_KEY)
  } catch {
    /* приватный режим: токен живёт только в памяти */
  }
}

// ─── новый чат ───
/**
 * «Начать заново» не создаёт чат на бэкенде: он появится с первым вопросом.
 * Пока вопроса нет, метка в sessionStorage говорит, что показывать историю не нужно, —
 * иначе после перезагрузки страницы снова откроется прошлый чат.
 */
const NEW_CHAT_KEY = 'buddy.newChat'
let newChatPending = readNewChat()

function readNewChat(): boolean {
  try {
    return sessionStorage.getItem(NEW_CHAT_KEY) === '1'
  } catch {
    return false
  }
}
function saveNewChat(value: boolean) {
  newChatPending = value
  try {
    if (value) sessionStorage.setItem(NEW_CHAT_KEY, '1')
    else sessionStorage.removeItem(NEW_CHAT_KEY)
  } catch {
    /* приватный режим: метка живёт только в памяти */
  }
}

export function hasSession(): boolean {
  return API_MODE === 'mock' || token !== null
}

/** Колбэк на 401: токен протух или отозван — App возвращает экран входа. */
export function onUnauthorized(cb: (() => void) | null) {
  unauthorized = cb
}

export function logout() {
  saveToken(null)
  saveNewChat(false)
  conversationId = null
  conversationVersion += 1
  historyCache = null
}

/**
 * Последняя показанная лента чата. Вкладка «Чат» при каждом открытии монтируется заново;
 * с кэшем сообщения видны сразу, без пустого экрана и прокрутки с начала ленты.
 */
let historyCache: ChatMessage[] | null = null
export function cachedHistory(): ChatMessage[] | null {
  return historyCache
}
export function rememberHistory(messages: ChatMessage[]) {
  historyCache = messages
}

// ─── HTTP ───
async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)
  let r: Response
  try {
    r = await fetch(BASE + path, { ...init, headers })
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e
    throw new ApiError(0, 'network')
  }
  if (r.status === 401 && path !== '/api/login') {
    logout()
    unauthorized?.()
  }
  if (!r.ok) throw new ApiError(r.status, await errorCode(r))
  return r.json() as Promise<T>
}

async function errorCode(r: Response): Promise<string> {
  try {
    const body = (await r.json()) as { error?: string }
    return body.error || `http_${r.status}`
  } catch {
    return `http_${r.status}`
  }
}

// ─── пользователь ───
interface ApiUser {
  id: number
  login: string
  full_name: string
  role: string
  team: string
  mentor?: string
}

const ROLE: Record<string, string> = { intern: 'Стажёр', mentor: 'Наставник', lead: 'Руководитель' }

const KINDS: readonly string[] = ['intern', 'mentor', 'lead'] satisfies UserKind[]

export function toUser(u: ApiUser): User {
  const [first = u.login, last = ''] = u.full_name.trim().split(/\s+/)
  return {
    name: first,
    short: last ? `${first} ${last[0]}.` : first,
    role: ROLE[u.role] ?? u.role,
    kind: KINDS.includes(u.role) ? (u.role as UserKind) : 'other',
  }
}

export function timeOf(iso: string | Date): string {
  return new Date(iso).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

export function now(): string {
  return timeOf(new Date())
}

let seq = 0
export function newId(): string {
  seq += 1
  return `m${seq}`
}

const delay = (ms: number) => new Promise<void>((res) => setTimeout(res, ms))

/**
 * UUID v4 для client_message_id. crypto.randomUUID есть только в защищённом контексте
 * (https или localhost), поэтому при открытии dev-сервера по LAN-IP нужен запасной вариант.
 */
function clientMessageId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16)
  })
}

/** Вход по логину тестовой учётки; без аргумента — демо-стажёр (кнопка «Войти через Avito ID»). */
export async function login(loginName: string = DEMO_LOGIN): Promise<User> {
  if (API_MODE === 'mock') {
    await delay(300)
    return mockUser()
  }
  const res = await request<{ token: string; user: ApiUser }>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ login: loginName.trim() }),
  })
  saveToken(res.token)
  saveNewChat(false)
  conversationId = null
  conversationVersion += 1
  historyCache = null // лента прошлого пользователя не должна мелькнуть у нового
  return toUser(res.user)
}

export async function fetchMe(): Promise<User> {
  if (API_MODE === 'mock') return mockUser()
  return toUser(await request<ApiUser>('/api/me'))
}

// ─── главная ───
interface ApiTask {
  id: number
  title: string
  status: 'todo' | 'in_progress' | 'review' | 'done'
  submitted_at?: string | null
  review_comment?: string | null
}
interface ApiChecklist {
  progress: { done: number; total: number; percent: number }
  next: ApiTask | null
  stages: { code: string; title: string; tasks: ApiTask[] }[]
}

const TASK_STATUS: Record<ApiTask['status'], FocusTask['status']> = {
  done: 'done',
  in_progress: 'work',
  review: 'review',
  todo: 'next',
}

/** Задача API → строка фокуса: комментарий наставника нужен только у возвращённой (in_progress). */
export function toFocusTask(t: ApiTask): FocusTask {
  const status = TASK_STATUS[t.status] ?? 'next'
  return {
    id: String(t.id),
    title: t.title,
    status,
    ...(status === 'review' && t.submitted_at ? { submittedAt: t.submitted_at } : {}),
    ...(status === 'work' && t.review_comment?.trim() ? { reviewComment: t.review_comment.trim() } : {}),
  }
}

interface ApiTeamMember {
  id: number
  full_name: string
  role: string
  is_my_mentor: boolean
}

/**
 * Главная: онбординг из GET /api/checklist, коллеги из GET /api/team, название команды из GET /api/me.
 * Фокус — до трёх задач: сначала начатые, затем отправленные на проверку (стажёр видит статус),
 * затем следующие по порядку; всё пройдено — последние три.
 * Пользователь для приветствия — из /api/me в App.
 */
export async function fetchHome(): Promise<Omit<HomeData, 'user'>> {
  if (API_MODE === 'mock') return mockHome()
  const [me, checklist, { members }] = await Promise.all([
    request<ApiUser>('/api/me'),
    request<ApiChecklist>('/api/checklist'),
    request<{ members: ApiTeamMember[] }>('/api/team'),
  ])
  const tasks = checklist.stages.flatMap((s) => s.tasks)
  const open = [
    ...tasks.filter((t) => t.status === 'in_progress'),
    ...tasks.filter((t) => t.status === 'review'),
    ...tasks.filter((t) => t.status === 'todo'),
  ]
  const focus = (open.length ? open : tasks.slice(-3)).slice(0, 3).map(toFocusTask)
  return {
    focus,
    onboarding: {
      done: checklist.progress.done,
      total: checklist.progress.total,
      nextStep: checklist.next?.title ?? 'Адаптация пройдена',
    },
    // свой наставник первым, дальше порядок бэкенда (лиды, наставники, стажёры)
    team: [...members]
      .sort((a, b) => Number(b.is_my_mentor) - Number(a.is_my_mentor))
      .map((m) => ({ id: String(m.id), name: m.full_name, role: m.role, isMyMentor: m.is_my_mentor })),
    teamName: me.team || 'Команда',
  }
}

// ─── апрув задач (docs/approvals-contract.md) ───
/** Стажёр отправляет задачу наставнику. Возвращает фактический статус задачи. */
export async function submitForReview(taskId: string): Promise<FocusTask> {
  if (API_MODE === 'mock') {
    await delay(250)
    mockSubmit(taskId)
    return { id: taskId, title: '', status: 'review', submittedAt: new Date().toISOString() }
  }
  const { task } = await request<{ task: ApiTask }>(`/api/checklist/tasks/${encodeURIComponent(taskId)}`, {
    method: 'PATCH',
    body: JSON.stringify({ status: 'review' }),
  })
  return toFocusTask(task)
}

interface ApiReview {
  task_id: number
  task_title: string
  task_description?: string
  stage_title?: string
  intern: { id: number; full_name: string }
  submitted_at: string
}

/** Запросы на проверку для наставника; старые сверху (порядок бэкенда). Не наставнику — пусто. */
export async function fetchReviews(): Promise<ReviewRequest[]> {
  if (API_MODE === 'mock') return mockReviews()
  const { reviews } = await request<{ reviews: ApiReview[] | null }>('/api/reviews')
  return (reviews ?? []).map((r) => ({
    taskId: String(r.task_id),
    taskTitle: r.task_title,
    taskDescription: r.task_description ?? '',
    stageTitle: r.stage_title ?? '',
    internId: String(r.intern.id),
    internName: r.intern.full_name,
    submittedAt: r.submitted_at,
  }))
}

interface ApiInternProgress {
  id: number
  full_name: string
  done: number
  total: number
  percent?: number
  in_review?: number
  stage?: string
}

/** Стажёры наставника с прогрессом адаптации (порядок бэкенда — по имени). */
export async function fetchInterns(): Promise<InternProgress[]> {
  if (API_MODE === 'mock') return mockInterns()
  const { interns } = await request<{ interns: ApiInternProgress[] | null }>('/api/interns')
  return (interns ?? []).map((i) => ({
    id: String(i.id),
    name: i.full_name,
    done: i.done,
    total: i.total,
    percent: i.percent ?? (i.total ? Math.round((i.done / i.total) * 100) : 0),
    inReview: i.in_review ?? 0,
    stage: i.stage ?? '',
  }))
}

/** Длина комментария при отклонении (после trim), как на бэкенде. */
export const REVIEW_COMMENT_MAX = 500

async function resolveReview(taskId: string, action: 'approve' | 'reject', body?: unknown): Promise<void> {
  if (API_MODE === 'mock') {
    await delay(250)
    if (!mockResolve(taskId)) throw new ApiError(404, 'not found')
    return
  }
  await request<{ task_id: number; status: string }>(`/api/reviews/${encodeURIComponent(taskId)}/${action}`, {
    method: 'POST',
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
}

/** Наставник принимает задачу → done. */
export function approveTask(taskId: string): Promise<void> {
  return resolveReview(taskId, 'approve')
}

/** Наставник возвращает задачу с комментарием → in_progress. Пустой комментарий не отправляется. */
export async function rejectTask(taskId: string, comment: string): Promise<void> {
  const text = comment.trim()
  if (!text || text.length > REVIEW_COMMENT_MAX) throw new ApiError(422, 'comment is required')
  return resolveReview(taskId, 'reject', { comment: text })
}

/** «N мин назад» для времени отправки на проверку. */
export function ago(iso: string, at: number = Date.now()): string {
  const min = Math.floor((at - new Date(iso).getTime()) / 60_000)
  if (!Number.isFinite(min)) return ''
  if (min < 1) return 'только что'
  if (min < 60) return `${min} мин назад`
  const h = Math.floor(min / 60)
  if (h < 24) return `${h} ч назад`
  return `${Math.floor(h / 24)} дн назад`
}

/** Текст ошибки отправки на проверку / принятия / отклонения. */
export function reviewErrorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 404) return 'Запрос не найден — возможно, его уже обработали.'
    if (e.status === 409) return e.code === 'task already approved' ? 'Задачу уже приняли.' : 'Задачу уже проверили.'
    if (e.status === 422) return `Напиши комментарий для стажёра (до ${REVIEW_COMMENT_MAX} символов).`
    if (e.status === 400) return 'Некорректная задача.'
  }
  return errorText(e, 'Не получилось сохранить. Попробуй ещё раз.')
}

// ─── чат ───
interface ApiConversation {
  id: number
  title: string
  updated_at: string
}
interface ApiMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

/**
 * История последнего чата пользователя. Пустой массив, если чатов ещё нет.
 * null — пока запрос шёл, пользователь отправил сообщение или начал новый чат:
 * результат устарел, показывать его и переключать текущий чат нельзя.
 */
export async function fetchHistory(): Promise<ChatMessage[] | null> {
  if (API_MODE === 'mock') return []
  // «Начать заново» без вопроса: новый чат ещё не создан на бэкенде,
  // и последний чат из списка — прошлый, его показывать не надо
  if (newChatPending) {
    conversationId = null
    historyCache = []
    return []
  }
  const version = conversationVersion
  const { conversations } = await request<{ conversations: ApiConversation[] }>('/api/conversations')
  const last = conversations[0] // Go отдаёт по updated_at DESC
  if (version !== conversationVersion) return null
  if (!last) {
    conversationId = null
    return []
  }
  conversationId = last.id
  const { messages } = await request<{ messages: ApiMessage[] }>(`/api/conversations/${last.id}/messages?limit=100`)
  if (version !== conversationVersion) return null
  return messages.map((m) => ({
    id: `s${m.id}`,
    role: m.role === 'user' ? 'user' : 'bot',
    text: m.content,
    at: timeOf(m.created_at),
  }))
}

/** «Начать заново»: следующий вопрос откроет новый чат на бэкенде. */
export function startNewChat() {
  conversationId = null
  conversationVersion += 1
  historyCache = []
  saveNewChat(true)
}

async function ensureConversation(firstMessage: string): Promise<number> {
  if (conversationId !== null) return conversationId
  const version = conversationVersion
  const title = firstMessage.length > 60 ? `${firstMessage.slice(0, 57)}…` : firstMessage
  const c = await request<ApiConversation>('/api/conversations', { method: 'POST', body: JSON.stringify({ title }) })
  // Пока создавался чат, могли нажать «Начать заново» или войти заново:
  // такой ответ уже не должен становиться текущим чатом.
  if (version === conversationVersion) {
    conversationId = c.id
    conversationVersion += 1 // загрузка истории, начатая до этого, уже не актуальна
    saveNewChat(false) // чат создан — история снова берётся с сервера
  }
  return c.id
}

/**
 * Отправляет вопрос и читает потоковый ответ агента.
 * Возвращает итоговое сообщение Buddy; при ошибке агента, обрыве потока или тишине
 * дольше IDLE_TIMEOUT_MS бросает ApiError (code: error-код агента, 'stream_cut' или 'timeout').
 */
export async function sendMessage(text: string, h: StreamHandlers = {}, idleMs = IDLE_TIMEOUT_MS): Promise<ChatMessage> {
  if (API_MODE === 'mock') {
    await delay(700)
    h.onText?.(BUDDY_REPLY)
    return { id: newId(), role: 'bot', text: BUDDY_REPLY, at: now() }
  }

  const cid = await ensureConversation(text)
  const ctrl = new AbortController()
  let timedOut = false
  let timer: ReturnType<typeof setTimeout> | undefined
  const arm = () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      timedOut = true
      ctrl.abort()
    }, idleMs)
  }
  const onOuterAbort = () => ctrl.abort()
  h.signal?.addEventListener('abort', onOuterAbort)

  let full = ''
  let parts = 0
  let finished = false
  let failure: string | null = null
  try {
    arm()
    const r = await fetch(`${BASE}/api/chat`, {
      method: 'POST',
      signal: ctrl.signal,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token ?? ''}` },
      body: JSON.stringify({ conversation_id: cid, client_message_id: clientMessageId(), content: text }),
    })
    if (r.status === 401) {
      logout()
      unauthorized?.()
    }
    if (!r.ok || !r.body) throw new ApiError(r.status, await errorCode(r))

    await readSse(r.body, ({ event, data }) => {
      arm()
      const d = (data ?? {}) as { text?: string; code?: string }
      switch (event) {
        case 'status':
          if (d.text) h.onStatus?.(d.text)
          break
        case 'message_start':
          // агент может ответить несколькими сообщениями (между ними — вызовы инструментов)
          if (parts > 0 && full) full += '\n\n'
          parts += 1
          break
        case 'text_delta':
          if (d.text) {
            full += d.text
            h.onText?.(full)
          }
          break
        case 'done':
          finished = true
          break
        case 'error':
          failure = d.code || 'agent_failed'
          break
      }
    })
  } catch (e) {
    if (e instanceof ApiError) throw e
    if (timedOut) throw new ApiError(0, 'timeout')
    if ((e as Error).name === 'AbortError') throw e
    throw new ApiError(0, 'network')
  } finally {
    clearTimeout(timer)
    h.signal?.removeEventListener('abort', onOuterAbort)
  }
  if (failure) throw new ApiError(0, failure)
  if (!finished) throw new ApiError(0, 'stream_cut')
  return { id: newId(), role: 'bot', text: full.trim(), at: now() }
}

/** Текст ошибки для пользователя. */
export function errorText(e: unknown, fallback = 'Не получилось получить ответ. Попробуй ещё раз.'): string {
  const code = e instanceof ApiError ? e.code : ''
  if (code === 'timeout') return 'Buddy долго не отвечает. Попробуй ещё раз.'
  // 409: в этом чате ещё идёт прошлый ответ (например, вкладку закрыли посреди ответа)
  if (e instanceof ApiError && e.status === 409)
    return 'Buddy ещё заканчивает прошлый ответ. Подожди минуту или нажми «Начать заново».'
  if (code === 'network' || code === 'agent unavailable' || code === 'http_502' || code === 'http_504')
    return 'Сервер недоступен. Проверь, что бэкенд запущен.'
  return fallback
}
