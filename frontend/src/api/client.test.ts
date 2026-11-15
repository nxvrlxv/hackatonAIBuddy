import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type Client = typeof import('./client')

const USER = { id: 3, login: 'sasha.intern', full_name: 'Саша Иванов', role: 'intern', team_id: 2, team: 'Путешествия' }

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function sse(frames: [string, unknown][]) {
  const text = frames.map(([e, d]) => `event: ${e}\ndata: ${JSON.stringify(d)}\n\n`).join('')
  return new Response(text, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

/** Поток, который молчит, пока запрос не отменят. */
function silentStream(signal: AbortSignal) {
  return new Response(
    new ReadableStream({
      start(c) {
        signal.addEventListener('abort', () => c.error(new DOMException('aborted', 'AbortError')))
      },
    }),
    { status: 200 },
  )
}

interface Call {
  url: string
  method: string
  auth: string | null
  body: unknown
}

let calls: Call[]
let routes: (url: string, init: RequestInit) => Response | Promise<Response>

beforeEach(() => {
  calls = []
  vi.resetModules()
  vi.stubGlobal('fetch', async (url: string, init: RequestInit = {}) => {
    const headers = new Headers(init.headers)
    calls.push({
      url,
      method: init.method ?? 'GET',
      auth: headers.get('Authorization'),
      body: init.body ? JSON.parse(String(init.body)) : undefined,
    })
    return routes(url, init)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

async function loadClient(): Promise<Client> {
  return import('./client')
}

/** Типовой бэкенд: вход, один пустой список чатов, создание чата, поток ответа. */
function backend(chatFrames: [string, unknown][] = [['done', { run_id: 'r1' }]]) {
  return (url: string): Response => {
    if (url === '/api/login') return json({ token: 'T1', user: USER })
    if (url === '/api/me') return json(USER)
    if (url === '/api/conversations') return json({ id: 7, title: 't', updated_at: '2026-11-13T10:00:00Z' }, 201)
    if (url === '/api/chat') return sse(chatFrames)
    return json({ error: 'not found' }, 404)
  }
}

describe('пользователь', () => {
  it('toUser: имя, подпись «Имя Ф.», роль по-русски и машинная роль', async () => {
    const { toUser } = await loadClient()
    expect(toUser(USER)).toEqual({ name: 'Саша', short: 'Саша И.', role: 'Стажёр', kind: 'intern' })
    expect(toUser({ ...USER, full_name: 'Анна', role: 'mentor' })).toEqual({
      name: 'Анна',
      short: 'Анна',
      role: 'Наставник',
      kind: 'mentor',
    })
    expect(toUser({ ...USER, role: 'lead' }).kind).toBe('lead')
    expect(toUser({ ...USER, role: 'admin' })).toMatchObject({ role: 'admin', kind: 'other' })
  })

  it('login отправляет тестовый логин и дальше ходит с токеном', async () => {
    routes = backend()
    const c = await loadClient()
    expect(c.hasSession()).toBe(false)
    const u = await c.login()
    expect(u.name).toBe('Саша')
    expect(calls[0]).toMatchObject({ url: '/api/login', method: 'POST', body: { login: 'sasha.intern' } })
    expect(c.hasSession()).toBe(true)
    await c.fetchMe()
    expect(calls[1]).toMatchObject({ url: '/api/me', auth: 'Bearer T1' })
  })

  it('неизвестный логин даёт ApiError с кодом сервера', async () => {
    routes = () => json({ error: 'invalid login' }, 401)
    const c = await loadClient()
    await expect(c.login()).rejects.toMatchObject({ status: 401, code: 'invalid login' })
  })

  it('401 на защищённой ручке сбрасывает сессию и зовёт onUnauthorized', async () => {
    routes = backend()
    const c = await loadClient()
    await c.login()
    routes = () => json({ error: 'invalid token' }, 401)
    const cb = vi.fn()
    c.onUnauthorized(cb)
    await expect(c.fetchMe()).rejects.toMatchObject({ code: 'invalid token' })
    expect(cb).toHaveBeenCalledOnce()
    expect(c.hasSession()).toBe(false)
  })

  it('сетевая ошибка превращается в код network', async () => {
    routes = () => {
      throw new TypeError('fetch failed')
    }
    const c = await loadClient()
    await expect(c.login()).rejects.toMatchObject({ code: 'network' })
  })

  it('ответ без JSON даёт код http_<статус>', async () => {
    routes = () => new Response('oops', { status: 502 })
    const c = await loadClient()
    await expect(c.login()).rejects.toMatchObject({ code: 'http_502' })
  })
})

describe('история', () => {
  it('берёт последний чат и переводит роли', async () => {
    routes = (url) => {
      if (url === '/api/conversations')
        return json({ conversations: [{ id: 9, title: 'новый', updated_at: '' }, { id: 4, title: 'старый', updated_at: '' }] })
      if (url === '/api/conversations/9/messages?limit=100')
        return json({
          messages: [
            { id: 1, role: 'user', content: 'Как получить доступ?', created_at: '2026-11-13T07:14:00Z' },
            { id: 2, role: 'assistant', content: 'Через сервис «Доступы».', created_at: '2026-11-13T07:15:00Z' },
          ],
        })
      return json({}, 404)
    }
    const c = await loadClient()
    const h = await c.fetchHistory()
    expect(h.map((m) => [m.role, m.text])).toEqual([
      ['user', 'Как получить доступ?'],
      ['bot', 'Через сервис «Доступы».'],
    ])
    expect(h[0].at).toMatch(/^\d\d:\d\d$/)
  })

  it('нет чатов — пустая история, первый вопрос создаёт чат', async () => {
    const chat = backend([
      ['message_start', { message_id: 'a' }],
      ['text_delta', { text: 'ok' }],
      ['done', {}],
    ])
    routes = (url, init) => (url === '/api/conversations' && !init.method ? json({ conversations: [] }) : chat(url))
    const c = await loadClient()
    expect(await c.fetchHistory()).toEqual([])
    await c.sendMessage('Привет')
    expect(calls.map((x) => `${x.method} ${x.url}`)).toEqual(['GET /api/conversations', 'POST /api/conversations', 'POST /api/chat'])
  })

  it('режим mock не ходит в сеть', async () => {
    vi.stubEnv('VITE_API_MODE', 'mock')
    routes = () => {
      throw new Error('сети быть не должно')
    }
    const c = await loadClient()
    expect(c.API_MODE).toBe('mock')
    expect(c.hasSession()).toBe(true)
    expect((await c.login()).name).toBe('Александр')
    expect(await c.fetchMe()).toMatchObject({ role: 'Стажёр', kind: 'intern' })
    expect(await c.fetchHistory()).toEqual([])
    const onText = vi.fn()
    const r = await c.sendMessage('?', { onText })
    expect(onText).toHaveBeenCalledWith(r.text)
    expect(calls).toHaveLength(0)
  })
})

describe('sendMessage', () => {
  it('создаёт чат, шлёт вопрос и собирает потоковый ответ', async () => {
    routes = backend([
      ['accepted', { run_id: 'r1' }],
      ['status', { text: 'Восстанавливаю контекст' }],
      ['message_start', { message_id: 'm1' }],
      ['text_delta', { text: 'Открой ' }],
      ['text_delta', { text: 'сервис «Доступы».' }],
      ['message_done', {}],
      ['warning', { code: 'index_pending' }],
      ['done', { run_id: 'r1' }],
    ])
    const c = await loadClient()
    await c.login()
    const texts: string[] = []
    const statuses: string[] = []
    const reply = await c.sendMessage('Как получить доступ к GitLab?', {
      onText: (t) => texts.push(t),
      onStatus: (s) => statuses.push(s),
    })
    expect(reply).toMatchObject({ role: 'bot', text: 'Открой сервис «Доступы».' })
    expect(texts).toEqual(['Открой ', 'Открой сервис «Доступы».'])
    expect(statuses).toEqual(['Восстанавливаю контекст'])

    const create = calls.find((x) => x.url === '/api/conversations')!
    expect(create).toMatchObject({ method: 'POST', auth: 'Bearer T1', body: { title: 'Как получить доступ к GitLab?' } })
    const chat = calls.find((x) => x.url === '/api/chat')!
    expect(chat.auth).toBe('Bearer T1')
    expect(chat.body).toMatchObject({ conversation_id: 7, content: 'Как получить доступ к GitLab?' })
    expect((chat.body as { client_message_id: string }).client_message_id).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('второй вопрос идёт в тот же чат, после startNewChat — в новый', async () => {
    routes = backend()
    const c = await loadClient()
    await c.login()
    await c.sendMessage('раз')
    await c.sendMessage('два')
    expect(calls.filter((x) => x.url === '/api/conversations')).toHaveLength(1)
    c.startNewChat()
    await c.sendMessage('три')
    expect(calls.filter((x) => x.url === '/api/conversations')).toHaveLength(2)
  })

  it('длинный первый вопрос обрезается в заголовке чата', async () => {
    routes = backend()
    const c = await loadClient()
    await c.login()
    await c.sendMessage('x'.repeat(100))
    const title = (calls.find((x) => x.url === '/api/conversations')!.body as { title: string }).title
    expect(title).toHaveLength(58)
    expect(title.endsWith('…')).toBe(true)
  })

  it('несколько сообщений агента склеиваются через пустую строку', async () => {
    routes = backend([
      ['message_start', { message_id: 'a' }],
      ['text_delta', { text: 'Ищу в базе.' }],
      ['message_start', { message_id: 'b' }],
      ['text_delta', { text: 'Нашёл.' }],
      ['done', {}],
    ])
    const c = await loadClient()
    await c.login()
    expect((await c.sendMessage('?')).text).toBe('Ищу в базе.\n\nНашёл.')
  })

  it('событие error даёт ApiError с кодом агента', async () => {
    routes = backend([
      ['message_start', { message_id: 'a' }],
      ['text_delta', { text: 'нач' }],
      ['error', { code: 'llm_unavailable' }],
    ])
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?')).rejects.toMatchObject({ code: 'llm_unavailable' })
  })

  it('error без кода — agent_failed', async () => {
    routes = backend([['error', {}]])
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?')).rejects.toMatchObject({ code: 'agent_failed' })
  })

  it('поток оборвался без done — stream_cut', async () => {
    routes = backend([['text_delta', { text: 'нач' }]])
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?')).rejects.toMatchObject({ code: 'stream_cut' })
  })

  it('агент молчит дольше таймаута — timeout', async () => {
    const base = backend()
    routes = (url, init) => (url === '/api/chat' ? silentStream(init.signal!) : base(url))
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?', {}, 30)).rejects.toMatchObject({ code: 'timeout' })
  })

  it('внешняя отмена пробрасывает AbortError', async () => {
    const base = backend()
    routes = (url, init) => (url === '/api/chat' ? silentStream(init.signal!) : base(url))
    const c = await loadClient()
    await c.login()
    const ctrl = new AbortController()
    const p = c.sendMessage('?', { signal: ctrl.signal }, 5_000)
    setTimeout(() => ctrl.abort(), 10)
    await expect(p).rejects.toMatchObject({ name: 'AbortError' })
  })

  it('прокси вернул 502 — ApiError с кодом сервера', async () => {
    const base = backend()
    routes = (url) => (url === '/api/chat' ? json({ error: 'agent unavailable' }, 502) : base(url))
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?')).rejects.toMatchObject({ status: 502, code: 'agent unavailable' })
  })

  it('401 на /api/chat сбрасывает сессию', async () => {
    const base = backend()
    routes = (url) => (url === '/api/chat' ? json({ error: 'invalid token' }, 401) : base(url))
    const c = await loadClient()
    await c.login()
    const cb = vi.fn()
    c.onUnauthorized(cb)
    await expect(c.sendMessage('?')).rejects.toMatchObject({ status: 401 })
    expect(cb).toHaveBeenCalledOnce()
    expect(c.hasSession()).toBe(false)
  })

  it('сеть упала посреди запроса — network', async () => {
    const base = backend()
    routes = (url) => {
      if (url === '/api/chat') throw new TypeError('fetch failed')
      return base(url)
    }
    const c = await loadClient()
    await c.login()
    await expect(c.sendMessage('?')).rejects.toMatchObject({ code: 'network' })
  })
})

describe('errorText', () => {
  it('понятные тексты для таймаута, недоступного сервера и прочего', async () => {
    const { ApiError, errorText } = await loadClient()
    expect(errorText(new ApiError(0, 'timeout'))).toMatch(/долго не отвечает/)
    expect(errorText(new ApiError(0, 'network'))).toMatch(/Сервер недоступен/)
    expect(errorText(new ApiError(502, 'agent unavailable'))).toMatch(/Сервер недоступен/)
    expect(errorText(new ApiError(0, 'agent_failed'))).toMatch(/Не получилось получить ответ/)
    expect(errorText(new Error('x'), 'Не получилось войти.')).toBe('Не получилось войти.')
  })
})

describe('главная', () => {
  const task = (id: number, title: string, status: string) => ({ id, title, description: '', status })
  const checklist = (tasks: ReturnType<typeof task>[], next: ReturnType<typeof task> | null) => ({
    progress: { done: tasks.filter((t) => t.status === 'done').length, total: tasks.length, percent: 0 },
    next,
    stages: [{ code: 'team', title: 'Команда', done: 0, total: tasks.length, tasks }],
  })

  it('онбординг из /api/checklist, коллеги из /api/team, название команды из /api/me', async () => {
    const tasks = [task(1, 'A', 'done'), task(2, 'B', 'todo'), task(3, 'C', 'in_progress'), task(4, 'D', 'todo'), task(5, 'E', 'todo')]
    routes = (url) => {
      if (url === '/api/me') return json({ ...USER, mentor: 'Анна Смирнова' })
      if (url === '/api/checklist') return json(checklist(tasks, tasks[2]))
      if (url === '/api/team')
        return json({
          members: [
            { id: 1, full_name: 'Игорь Лидов', role: 'lead', is_my_mentor: false },
            { id: 2, full_name: 'Анна Смирнова', role: 'mentor', is_my_mentor: true },
          ],
        })
      return backend()(url)
    }
    const c = await loadClient()
    await c.login()
    const home = await c.fetchHome()
    expect(home.focus).toEqual([
      { id: '3', title: 'C', status: 'work' },
      { id: '2', title: 'B', status: 'next' },
      { id: '4', title: 'D', status: 'next' },
    ])
    expect(home.onboarding).toEqual({ done: 1, total: 5, nextStep: 'C' })
    // свой наставник поднимается первым
    expect(home.team).toEqual([
      { id: '2', name: 'Анна Смирнова', role: 'mentor', isMyMentor: true },
      { id: '1', name: 'Игорь Лидов', role: 'lead', isMyMentor: false },
    ])
    expect(home.teamName).toBe('Путешествия')
    expect(calls.filter((x) => x.url === '/api/checklist')[0].auth).toBe('Bearer T1')
  })

  it('всё пройдено — последние задачи и финальный шаг; пустая команда и без названия', async () => {
    const tasks = [1, 2, 3, 4].map((i) => task(i, `T${i}`, 'done'))
    routes = (url) => {
      if (url === '/api/me') return json({ ...USER, team: '' })
      if (url === '/api/checklist') return json(checklist(tasks, null))
      if (url === '/api/team') return json({ members: [] })
      return backend()(url)
    }
    const c = await loadClient()
    const home = await c.fetchHome()
    expect(home.focus.map((f) => f.id)).toEqual(['2', '3', '4'])
    expect(home.focus.every((f) => f.status === 'done')).toBe(true)
    expect(home.onboarding.nextStep).toBe('Адаптация пройдена')
    expect(home.team).toEqual([])
    expect(home.teamName).toBe('Команда')
  })

  it('режим mock отдаёт заглушку без сети', async () => {
    vi.stubEnv('VITE_API_MODE', 'mock')
    const c = await loadClient()
    expect((await c.fetchHome()).onboarding.total).toBe(7)
    expect(calls).toHaveLength(0)
  })
})

describe('409 — прошлый ответ ещё идёт', () => {
  it('понятный текст вместо «попробуй ещё раз»', async () => {
    routes = (url) => (url === '/api/chat' ? json({ error: 'storage_rejected' }, 409) : backend()(url))
    const c = await loadClient()
    await c.login()
    const err = await c.sendMessage('привет').catch((e) => e)
    expect(err).toBeInstanceOf(c.ApiError)
    expect(c.errorText(err)).toContain('заканчивает прошлый ответ')
  })
})

describe('апрув задач', () => {
  const stage = (tasks: unknown[]) => ({
    progress: { done: 0, total: tasks.length, percent: 0 },
    next: null,
    stages: [{ code: 'access', title: 'Доступы', done: 0, total: tasks.length, tasks }],
  })
  const homeRoutes = (tasks: unknown[]) => (url: string) => {
    if (url === '/api/checklist') return json(stage(tasks))
    if (url === '/api/team') return json({ members: [] })
    return backend()(url)
  }

  it('review и возвращённая задача: статусы, submitted_at, комментарий; порядок фокуса', async () => {
    routes = homeRoutes([
      { id: 1, title: 'A', status: 'todo', submitted_at: null, review_comment: null },
      { id: 2, title: 'B', status: 'review', submitted_at: '2026-11-15T10:12:00Z', review_comment: null },
      { id: 3, title: 'C', status: 'in_progress', review_comment: ' Нет доступа к репозиторию ' },
      { id: 4, title: 'D', status: 'done', completed_at: '2026-11-15T09:00:00Z' },
    ])
    const c = await loadClient()
    const { focus } = await c.fetchHome()
    // начатые → на проверке → не начатые
    expect(focus).toEqual([
      { id: '3', title: 'C', status: 'work', reviewComment: 'Нет доступа к репозиторию' },
      { id: '2', title: 'B', status: 'review', submittedAt: '2026-11-15T10:12:00Z' },
      { id: '1', title: 'A', status: 'next' },
    ])
  })

  it('комментарий виден только у in_progress, submitted_at — только у review', async () => {
    const { toFocusTask } = await loadClient()
    expect(toFocusTask({ id: 1, title: 'X', status: 'done', submitted_at: '2026', review_comment: 'старый' })).toEqual({
      id: '1',
      title: 'X',
      status: 'done',
    })
    expect(toFocusTask({ id: 2, title: 'Y', status: 'in_progress', review_comment: '   ' })).toEqual({ id: '2', title: 'Y', status: 'work' })
  })

  it('все задачи на проверке — фокус показывает их', async () => {
    routes = homeRoutes([1, 2].map((id) => ({ id, title: `T${id}`, status: 'review', submitted_at: '2026-11-15T10:00:00Z' })))
    const c = await loadClient()
    expect((await c.fetchHome()).focus.map((f) => f.status)).toEqual(['review', 'review'])
  })

  it('submitForReview: PATCH со статусом review, ответ — фактический статус', async () => {
    routes = (url, init) => {
      if (url === '/api/checklist/tasks/42' && init.method === 'PATCH')
        return json({
          task: { id: 42, title: 'Доступ к GitLab', status: 'review', submitted_at: '2026-11-15T10:12:00Z', review_comment: null },
          progress: { done: 1, total: 7, percent: 14 },
        })
      return backend()(url)
    }
    const c = await loadClient()
    await c.login()
    const t = await c.submitForReview('42')
    expect(t).toEqual({ id: '42', title: 'Доступ к GitLab', status: 'review', submittedAt: '2026-11-15T10:12:00Z' })
    expect(calls.at(-1)).toMatchObject({ method: 'PATCH', auth: 'Bearer T1', body: { status: 'review' } })
  })

  it('submitForReview: принятую задачу менять нельзя — 409', async () => {
    routes = () => json({ error: 'task already approved' }, 409)
    const c = await loadClient()
    const err = await c.submitForReview('42').catch((e) => e)
    expect(err).toMatchObject({ status: 409, code: 'task already approved' })
    expect(c.reviewErrorText(err)).toBe('Задачу уже приняли.')
  })

  it('fetchReviews: маппинг запросов, порядок бэкенда', async () => {
    routes = (url) =>
      url === '/api/reviews'
        ? json({
            reviews: [
              {
                task_id: 42,
                task_title: 'Доступ к GitLab',
                task_description: 'Оформи заявку…',
                stage_title: 'Доступы и инструменты',
                intern: { id: 3, full_name: 'Саша Иванов' },
                submitted_at: '2026-11-15T10:12:00Z',
              },
              { task_id: 7, task_title: 'Окружение', intern: { id: 4, full_name: 'Дима Петров' }, submitted_at: '2026-11-15T11:00:00Z' },
            ],
          })
        : backend()(url)
    const c = await loadClient()
    expect(await c.fetchReviews()).toEqual([
      {
        taskId: '42',
        taskTitle: 'Доступ к GitLab',
        taskDescription: 'Оформи заявку…',
        stageTitle: 'Доступы и инструменты',
        internId: '3',
        internName: 'Саша Иванов',
        submittedAt: '2026-11-15T10:12:00Z',
      },
      {
        taskId: '7',
        taskTitle: 'Окружение',
        taskDescription: '',
        stageTitle: '',
        internId: '4',
        internName: 'Дима Петров',
        submittedAt: '2026-11-15T11:00:00Z',
      },
    ])
  })

  it('fetchInterns: маппинг прогресса стажёров, процент считается, если бэкенд не прислал', async () => {
    routes = (url) =>
      url === '/api/interns'
        ? json({
            interns: [
              { id: 3, full_name: 'Саша Иванов', done: 5, total: 11, percent: 45, in_review: 1, stage: 'Доступы и инструменты' },
              { id: 13, full_name: 'Дмитрий Орлов', done: 1, total: 4 },
            ],
          })
        : backend()(url)
    const c = await loadClient()
    expect(await c.fetchInterns()).toEqual([
      { id: '3', name: 'Саша Иванов', done: 5, total: 11, percent: 45, inReview: 1, stage: 'Доступы и инструменты' },
      { id: '13', name: 'Дмитрий Орлов', done: 1, total: 4, percent: 25, inReview: 0, stage: '' },
    ])
  })

  it('fetchInterns: null и пустой total — без деления на ноль', async () => {
    routes = () => json({ interns: null })
    let c = await loadClient()
    expect(await c.fetchInterns()).toEqual([])
    vi.resetModules()
    routes = () => json({ interns: [{ id: 1, full_name: 'X', done: 0, total: 0 }] })
    c = await loadClient()
    expect((await c.fetchInterns())[0].percent).toBe(0)
  })

  it('fetchInterns: 403 у не наставника — ошибка наружу', async () => {
    routes = () => json({ error: 'forbidden' }, 403)
    const c = await loadClient()
    await expect(c.fetchInterns()).rejects.toMatchObject({ status: 403 })
  })

  it('fetchReviews: не наставнику — пустой список (в т.ч. null)', async () => {
    routes = () => json({ reviews: null })
    const c = await loadClient()
    expect(await c.fetchReviews()).toEqual([])
  })

  it('approveTask: POST без тела', async () => {
    routes = (url) => (url === '/api/reviews/42/approve' ? json({ task_id: 42, status: 'done' }) : backend()(url))
    const c = await loadClient()
    await c.login()
    await c.approveTask('42')
    expect(calls.at(-1)).toMatchObject({ url: '/api/reviews/42/approve', method: 'POST', auth: 'Bearer T1', body: undefined })
  })

  it('rejectTask: POST с обрезанным комментарием', async () => {
    routes = (url) => (url === '/api/reviews/42/reject' ? json({ task_id: 42, status: 'in_progress' }) : backend()(url))
    const c = await loadClient()
    await c.rejectTask('42', '  Нет доступа к репозиторию команды \n')
    expect(calls.at(-1)).toMatchObject({ method: 'POST', body: { comment: 'Нет доступа к репозиторию команды' } })
  })

  it('rejectTask: пустой или слишком длинный комментарий не уходит на сервер — 422', async () => {
    routes = () => {
      throw new Error('сети быть не должно')
    }
    const c = await loadClient()
    await expect(c.rejectTask('42', '   ')).rejects.toMatchObject({ status: 422, code: 'comment is required' })
    await expect(c.rejectTask('42', 'x'.repeat(c.REVIEW_COMMENT_MAX + 1))).rejects.toMatchObject({ status: 422 })
    expect(calls).toHaveLength(0)
  })

  it('ошибки сервера: 404, 409, 422, 400 — понятные тексты', async () => {
    const c = await loadClient()
    routes = () => json({ error: 'not found' }, 404)
    const e404 = await c.approveTask('1').catch((e) => e)
    expect(e404).toMatchObject({ status: 404, code: 'not found' })
    expect(c.reviewErrorText(e404)).toMatch(/не найден/)

    routes = () => json({ error: 'task is not in review' }, 409)
    const e409 = await c.rejectTask('1', 'нет').catch((e) => e)
    expect(e409).toMatchObject({ status: 409, code: 'task is not in review' })
    expect(c.reviewErrorText(e409)).toBe('Задачу уже проверили.')

    routes = () => json({ error: 'comment is required' }, 422)
    const e422 = await c.rejectTask('1', 'нет').catch((e) => e)
    expect(c.reviewErrorText(e422)).toMatch(/комментарий/)

    routes = () => json({ error: 'invalid task id' }, 400)
    expect(c.reviewErrorText(await c.approveTask('x').catch((e) => e))).toBe('Некорректная задача.')

    routes = () => {
      throw new TypeError('fetch failed')
    }
    expect(c.reviewErrorText(await c.approveTask('1').catch((e) => e))).toMatch(/Сервер недоступен/)
    expect(c.reviewErrorText(new Error('x'))).toMatch(/Не получилось сохранить/)
  })

  it('ago: минуты, часы, дни', async () => {
    const { ago } = await loadClient()
    const at = Date.parse('2026-11-15T12:00:00Z')
    expect(ago('2026-11-15T11:59:40Z', at)).toBe('только что')
    expect(ago('2026-11-15T11:48:00Z', at)).toBe('12 мин назад')
    expect(ago('2026-11-15T09:00:00Z', at)).toBe('3 ч назад')
    expect(ago('2026-11-13T12:00:00Z', at)).toBe('2 дн назад')
    expect(ago('мусор', at)).toBe('')
  })

  it('режим mock: стажёр отправляет задачу, наставник без ?as=mentor запросов не видит', async () => {
    vi.stubEnv('VITE_API_MODE', 'mock')
    routes = () => {
      throw new Error('сети быть не должно')
    }
    const c = await loadClient()
    const before = await c.fetchHome()
    const next = before.focus.find((t) => t.status === 'next')!
    expect(await c.submitForReview(next.id)).toMatchObject({ status: 'review' })
    expect((await c.fetchHome()).focus.find((t) => t.id === next.id)?.status).toBe('review')
    expect(await c.fetchReviews()).toEqual([])
    expect(await c.fetchInterns()).toEqual([])
    await expect(c.approveTask('42')).rejects.toMatchObject({ status: 404 })
    expect(calls).toHaveLength(0)
  })

  it('режим mock: наставник принимает и отклоняет запросы', async () => {
    vi.stubEnv('VITE_API_MODE', 'mock')
    vi.stubGlobal('location', { search: '?as=mentor' })
    const c = await loadClient()
    expect((await c.fetchMe()).kind).toBe('mentor')
    expect((await c.fetchHome()).onboarding.total).toBe(0)
    const list = await c.fetchReviews()
    expect(list.length).toBeGreaterThan(1)
    await c.approveTask(list[0].taskId)
    await c.rejectTask(list[1].taskId, 'доработай')
    // «на проверке» у стажёров считается по оставшимся запросам
    const left = list.slice(2)
    for (const i of await c.fetchInterns()) expect(i.inReview).toBe(left.filter((r) => r.internId === i.id).length)
    expect((await c.fetchReviews()).map((r) => r.taskId)).toEqual(list.slice(2).map((r) => r.taskId))
    await expect(c.approveTask(list[0].taskId)).rejects.toMatchObject({ status: 404 })
  })
})
