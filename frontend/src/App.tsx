import { useEffect, useState } from 'react'
import { Chat } from './Chat'
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
    <Shell tab={tab} onTab={setTab} user={user}>
      {tab === 'main' ? <Home user={user} openChat={() => setTab('chat')} /> : <Chat user={user} />}
    </Shell>
  )
}
