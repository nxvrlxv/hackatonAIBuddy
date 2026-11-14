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
  it('toUser: имя, подпись «Имя Ф.» и роль по-русски', async () => {
    const { toUser } = await loadClient()
    expect(toUser(USER)).toEqual({ name: 'Саша', short: 'Саша И.', role: 'Стажёр' })
    expect(toUser({ ...USER, full_name: 'Анна', role: 'mentor' })).toEqual({ name: 'Анна', short: 'Анна', role: 'Наставник' })
    expect(toUser({ ...USER, role: 'admin' }).role).toBe('admin')
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
    expect((await c.fetchMe()).role).toBe('Стажёр')
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
