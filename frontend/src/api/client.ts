import { BUDDY_REPLY, HOME, MOCK_USER } from './mock'
import { readSse } from './sse'
import type { ChatMessage, HomeData, StreamHandlers, User } from './types'

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
 */

const env = import.meta.env
export const API_MODE: 'api' | 'mock' = env.VITE_API_MODE === 'mock' ? 'mock' : 'api'
const BASE = String(env.VITE_API_BASE ?? '').replace(/\/$/, '')
/** Вход имитирует корпоративный SSO: тестовая учётка без пароля (см. migrations/0003_initData). */
const DEMO_LOGIN = String(env.VITE_DEMO_LOGIN || 'sasha.intern')
/** Если агент молчит дольше, запрос обрывается и показывается «Повторить». */
export const IDLE_TIMEOUT_MS = 30_000

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

export function hasSession(): boolean {
  return API_MODE === 'mock' || token !== null
}

/** Колбэк на 401: токен протух или отозван — App возвращает экран входа. */
export function onUnauthorized(cb: (() => void) | null) {
  unauthorized = cb
}

export function logout() {
  saveToken(null)
  conversationId = null
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

const ROLE: Record<string, string> = { intern: 'Стажёр', mentor: 'Наставник' }

export function toUser(u: ApiUser): User {
  const [first = u.login, last = ''] = u.full_name.trim().split(/\s+/)
  return { name: first, short: last ? `${first} ${last[0]}.` : first, role: ROLE[u.role] ?? u.role }
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

export async function login(): Promise<User> {
  if (API_MODE === 'mock') {
    await delay(300)
    return MOCK_USER
  }
  const res = await request<{ token: string; user: ApiUser }>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ login: DEMO_LOGIN }),
  })
  saveToken(res.token)
  conversationId = null
  return toUser(res.user)
}

export async function fetchMe(): Promise<User> {
  if (API_MODE === 'mock') return MOCK_USER
  return toUser(await request<ApiUser>('/api/me'))
}

/** TODO(backend): заменить на GET /api/home, когда появятся задачи и контакты. Пользователь — из /api/me (App). */
export async function fetchHome(): Promise<Omit<HomeData, 'user'>> {
  return HOME
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

/** История последнего чата пользователя. Пустой массив, если чатов ещё нет. */
export async function fetchHistory(): Promise<ChatMessage[]> {
  if (API_MODE === 'mock') return []
  const { conversations } = await request<{ conversations: ApiConversation[] }>('/api/conversations')
  const last = conversations[0] // Go отдаёт по updated_at DESC
  conversationId = last ? last.id : null
  if (!last) return []
  const { messages } = await request<{ messages: ApiMessage[] }>(`/api/conversations/${last.id}/messages?limit=100`)
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
}

async function ensureConversation(firstMessage: string): Promise<number> {
  if (conversationId !== null) return conversationId
  const title = firstMessage.length > 60 ? `${firstMessage.slice(0, 57)}…` : firstMessage
  const c = await request<ApiConversation>('/api/conversations', { method: 'POST', body: JSON.stringify({ title }) })
  conversationId = c.id
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
      body: JSON.stringify({ conversation_id: cid, client_message_id: crypto.randomUUID(), content: text }),
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
  if (code === 'network' || code === 'agent unavailable' || code === 'http_502' || code === 'http_504')
    return 'Сервер недоступен. Проверь, что бэкенд запущен.'
  return fallback
}
