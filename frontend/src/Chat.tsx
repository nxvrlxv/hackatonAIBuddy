import { useEffect, useRef, useState } from 'react'
import { fetchHistory, newId, now, sendMessage } from './api/client'
import type { ChatMessage } from './api/types'
import { Avatar, Icon, Mascot } from './ui'
import type { User } from './api/types'

export function Chat({ user }: { user: User }) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState('')
  const [pending, setPending] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    let alive = true
    fetchHistory().then((h) => alive && setMessages(h))
    return () => {
      alive = false
    }
  }, [])

  // автопрокрутка вниз при новом сообщении и во время «печати»
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages, pending])

  // авто-высота textarea
  useEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 132)}px`
  }, [draft])

  async function submit(text: string) {
    const value = text.trim()
    if (!value || pending) return

    setMessages((prev) => [...prev, { id: newId(), role: 'user', text: value, at: now() }])
    setDraft('')
    setPending(true)

    try {
      const reply = await sendMessage(value)
      setMessages((prev) => [...prev, reply])
    } finally {
      setPending(false)
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter — отправить, Shift+Enter — перенос строки
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void submit(draft)
    }
  }

  const empty = messages.length === 0 && !pending

  return (
    <>
      <header className="page-head screen-in">
        <h1 className="page-title">AI-чат</h1>
        <p className="page-sub chat-title-sub">Задавай вопросы о процессах, инструментах и первой неделе.</p>
        <div className="head-art chat" aria-hidden="true">
          <i className="blob-green" />
          <i className="blob-pink" />
          <i className="blob-blue" />
          <Mascot variant="wave" className="mascot" />
        </div>
      </header>

      <section className="card chat-card">
        <div className="chat-top">
          <img src="/img/buddy-avatar.png" alt="" />
          <div>
            <b>Buddy</b>
            <span>
              <i className="dot-online" />
              {pending ? 'печатает…' : 'онлайн'}
            </span>
          </div>
          {messages.length > 0 && (
            <button className="chat-clear" onClick={() => setMessages([])}>
              Начать заново
            </button>
          )}
        </div>

        <div className="chat-scroll" ref={scrollRef}>
          {empty ? (
            <div className="chat-empty">
              <div className="chat-empty-head">
                <img className="chat-empty-avatar" src="/img/buddy-avatar.png" alt="" />
                <h2>Чем помочь?</h2>
              </div>
              <p>Спроси про доступы, процессы, онбординг или отгулы — отвечу по базе знаний команды.</p>
            </div>
          ) : (
            <>
              {messages.map((m) =>
                m.role === 'user' ? (
                  <div className="msg msg-user" key={m.id}>
                    <Avatar size={34} />
                    <div className="msg-body">
                      <div className="msg-meta">
                        <b>{user.name}</b> · {m.at}
                      </div>
                      <div className="bubble bubble-user">{m.text}</div>
                    </div>
                  </div>
                ) : (
                  <div className="msg" key={m.id}>
                    <img className="msg-avatar" src="/img/buddy-avatar.png" alt="" />
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
                  <img className="msg-avatar" src="/img/buddy-avatar.png" alt="" />
                  <div className="msg-body">
                    <div className="msg-meta">
                      <b>Buddy</b> · печатает…
                    </div>
                    <div className="bubble bubble-bot">
                      <span className="typing">
                        <i />
                        <i />
                        <i />
                      </span>
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
              void submit(draft)
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
