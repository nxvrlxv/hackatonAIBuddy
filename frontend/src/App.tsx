import { useEffect, useState } from 'react'
import { Chat } from './Chat'
import { Help } from './Help'
import { Home } from './Home'
import { Login } from './Login'
import { Shell, type Tab } from './Shell'
import { fetchMe, hasSession, logout, onUnauthorized } from './api/client'
import type { User } from './api/types'

/** ?s=main|chat открывает нужную вкладку сразу после входа (удобно при разработке). */
function initialTab(): Tab | null {
  const s = new URLSearchParams(location.search).get('s')
  return s === 'main' || s === 'chat' ? s : null
}

export default function App() {
  const [user, setUser] = useState<User | null>(null)
  // есть сохранённый токен — сначала пробуем восстановить сессию, экран входа не мигает
  const [restoring, setRestoring] = useState(() => hasSession())
  const [tab, setTab] = useState<Tab>(() => initialTab() ?? 'main')
  const [help, setHelp] = useState(false)
  /** Вопрос-пример из «Что умеет Buddy»: чат отправит его сам. */
  const [question, setQuestion] = useState<string>()
  /** Заготовка из главной (клик по задаче): встаёт в поле ввода, отправляет пользователь. */
  const [prefill, setPrefill] = useState<string>()

  useEffect(() => {
    onUnauthorized(() => setUser(null))
    return () => onUnauthorized(null)
  }, [])

  useEffect(() => {
    if (!restoring) return
    let alive = true
    fetchMe()
      .then((u) => alive && setUser(u))
      .catch(() => logout())
      .finally(() => alive && setRestoring(false))
    return () => {
      alive = false
    }
  }, [restoring])

  if (restoring) return null
  if (!user) return <Login onEnter={setUser} />

  return (
    <Shell
      tab={tab}
      onTab={setTab}
      user={user}
      onHelp={() => setHelp(true)}
      onLogout={() => {
        logout()
        setUser(null)
        setTab('main')
      }}
    >
      {/* Обе вкладки смонтированы всегда, видна одна. Иначе уход на главную размонтирует чат
          и обрывает ответ, который ещё печатается (а бэкенд считает ход незавершённым). */}
      <div className="pane screen-in" hidden={tab !== 'main'}>
        <Home
          user={user}
          active={tab === 'main'}
          openChat={(draft) => {
            setPrefill(draft)
            setTab('chat')
          }}
        />
      </div>
      <div className="pane screen-in" hidden={tab !== 'chat'}>
        <Chat user={user} active={tab === 'chat'} question={question}
          onQuestionSent={() => setQuestion(undefined)}
          prefill={prefill}
          onPrefilled={() => setPrefill(undefined)}
        />
      </div>
      {help && (
        <Help
          onClose={() => setHelp(false)}
          onAsk={(q) => {
            setHelp(false)
            setQuestion(q)
            setTab('chat')
          }}
        />
      )}
    </Shell>
  )
}
