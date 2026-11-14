import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { cachedHistory, errorText, fetchHistory, newId, now, rememberHistory, sendMessage, startNewChat } from './api/client'
import type { ChatMessage } from './api/types'
import { Avatar, Icon } from './ui'
import type { User } from './api/types'

export function Chat({ user }: { user: User }) {
  const [messages, setMessages] = useState<ChatMessage[]>(() => cachedHistory() ?? [])
  /** История получена (или взята из кэша): до этого лента пустая, без заглушки «спроси меня». */
  const [loaded, setLoaded] = useState(() => cachedHistory() !== null)
  /** Сообщения, которые уже были в ленте при открытии: они появляются без анимации «всплытия». */
  const [shown, setShown] = useState<ReadonlySet<string>>(() => new Set((cachedHistory() ?? []).map((m) => m.id)))
  const [draft, setDraft] = useState('')
  const [pending, setPending] = useState(false)
  /** Текст ответа, который ещё печатается (поток SSE). */
  const [partial, setPartial] = useState('')
  /** Служебный статус агента во время ответа. */
  const [status, setStatus] = useState('')
  /** Ошибка последнего вопроса: текст и вопрос для «Повторить». */
  const [failed, setFailed] = useState<{ text: string; question: string } | null>(null)
  const [historyError, setHistoryError] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    let alive = true
    fetchHistory()
      .then((h) => {
        if (!alive) return
        setShown((prev) => new Set([...prev, ...h.map((m) => m.id)]))
        // та же лента, что в кэше: не перерисовываем, иначе сообщения мигнут
        setMessages((prev) => (sameFeed(prev, h) ? prev : h))
        setLoaded(true)
      })
      .catch((e) => {
        if (!alive) return
        setHistoryError(errorText(e, 'Не получилось загрузить историю чата.'))
        setLoaded(true)
      })
    return () => {
      alive = false
      abortRef.current?.abort()
    }
  }, [])

  useEffect(() => {
    if (loaded) rememberHistory(messages)
  }, [loaded, messages])

  // Автопрокрутка вниз до отрисовки кадра. При открытии вкладки лента сразу стоит внизу
  // (без плавной прокрутки от первых сообщений), дальше новые сообщения доезжают плавно.
  const settled = useRef(false)
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el || !loaded) return
    if (!settled.current) {
      el.style.scrollBehavior = 'auto'
      el.scrollTop = el.scrollHeight
      el.style.scrollBehavior = ''
      settled.current = true
      return
    }
    el.scrollTop = el.scrollHeight
  }, [loaded, messages, pending, partial, failed])

  // авто-высота textarea
  useEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 132)}px`
  }, [draft])

  async function ask(question: string) {
    const ctrl = new AbortController()
    abortRef.current = ctrl
    setPending(true)
    setPartial('')
    setStatus('')
    setFailed(null)
    try {
      const reply = await sendMessage(question, { onText: setPartial, onStatus: setStatus, signal: ctrl.signal })
      setMessages((prev) => [...prev, reply])
    } catch (e) {
      if ((e as Error).name === 'AbortError') return
      setFailed({ text: errorText(e), question })
    } finally {
      if (abortRef.current === ctrl) {
        abortRef.current = null
        setPending(false)
        setPartial('')
        setStatus('')
      }
    }
  }

  function submit(text: string) {
    const value = text.trim()
    if (!value || pending) return
    setMessages((prev) => [...prev, { id: newId(), role: 'user', text: value, at: now() }])
    setDraft('')
    void ask(value)
  }

  function restart() {
    abortRef.current?.abort()
    abortRef.current = null
    startNewChat()
    setMessages([])
    setFailed(null)
    setPending(false)
    setPartial('')
    setStatus('')
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter — отправить, Shift+Enter — перенос строки
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit(draft)
    }
  }

  const empty = loaded && messages.length === 0 && !pending && !failed

  return (
    <>
      <header className="page-head">
        <h1 className="page-title">AI-чат</h1>
        <p className="page-sub chat-sub">Задавай вопросы о процессах, инструментах и первой неделе.</p>
      </header>

      <section className="card chat-card">
        <div className="chat-top">
          <img src="/img/buddy-avatar.webp" alt="" />
          <div>
            <b>Buddy</b>
            <span>
              <i className="dot-online" />
              {pending ? status || 'печатает…' : 'онлайн'}
            </span>
          </div>
          {(messages.length > 0 || failed) && (
            <button className="chat-clear" onClick={restart}>
              Начать заново
            </button>
          )}
        </div>

        <div className="chat-scroll" ref={scrollRef}>
          {empty ? (
            <div className="chat-empty">
              <div className="chat-empty-head">
                <img className="chat-empty-avatar" src="/img/buddy-avatar.webp" alt="" />
                <h2>Чем помочь?</h2>
              </div>
              <p>Спроси про доступы, процессы, онбординг или отгулы - отвечу по базе знаний команды.</p>
              {historyError && <p className="chat-error-text">{historyError}</p>}
            </div>
          ) : (
            <>
              {messages.map((m) =>
                m.role === 'user' ? (
                  <div className={`msg msg-user${shown.has(m.id) ? ' msg-static' : ''}`} key={m.id}>
                    <Avatar size={28} />
                    <div className="msg-body">
                      <div className="msg-meta">
                        {m.at} · <b>{user.name}</b>
                      </div>
                      <div className="bubble bubble-user">{m.text}</div>
                    </div>
                  </div>
                ) : (
                  <div className={`msg${shown.has(m.id) ? ' msg-static' : ''}`} key={m.id}>
                    <img className="msg-avatar" src="/img/buddy-avatar.webp" alt="" />
                    <div className="msg-body">
                      <div className="msg-meta">
                        <b>Buddy</b> · {m.at}
                      </div>
                      <div className="bubble bubble-bot">{m.text}</div>
                    </div>
                  </div>
                ),
              )}
              {pending && (
                <div className="msg">
                  <img className="msg-avatar" src="/img/buddy-avatar.webp" alt="" />
                  <div className="msg-body">
                    <div className="msg-meta">
                      <b>Buddy</b> · печатает…
                    </div>
                    <div className="bubble bubble-bot">
                      {partial || (
                        <span className="typing">
                          <i />
                          <i />
                          <i />
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              )}
              {failed && !pending && (
                <div className="msg">
                  <img className="msg-avatar" src="/img/buddy-avatar.webp" alt="" />
                  <div className="msg-body">
                    <div className="bubble bubble-bot bubble-error" role="alert">
                      {failed.text}
                      <button className="retry-btn" onClick={() => void ask(failed.question)}>
                        Повторить
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        <div className="chat-foot">
          <div className="info">
            <Icon name="info" size={18} className="info-ico" />
            <span>Если ответа нет в базе, Buddy предложит добавить вопрос в повестку встречи с наставником.</span>
          </div>

          <form
            className="composer"
            onSubmit={(e) => {
              e.preventDefault()
              submit(draft)
            }}
          >
            <textarea
              ref={inputRef}
              rows={1}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder="Спроси Buddy о работе, процессе или онбординге..."
              aria-label="Сообщение"
            />
            <button className="send-btn" type="submit" disabled={!draft.trim() || pending} aria-label="Отправить">
              <Icon name="send" size={20} />
            </button>
          </form>
        </div>
      </section>
    </>
  )
}

/** Одинаковые ленты: те же авторы и тексты в том же порядке (id у локальных и серверных разные). */
function sameFeed(a: ChatMessage[], b: ChatMessage[]): boolean {
  return a.length === b.length && a.every((m, i) => m.role === b[i].role && m.text === b[i].text)
}
