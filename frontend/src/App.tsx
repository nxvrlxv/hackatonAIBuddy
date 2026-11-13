import { useEffect, useState } from 'react'
import { Chat } from './Chat'
import { Home } from './Home'
import { Login } from './Login'
import { Shell, type Tab } from './Shell'
import { fetchMe } from './api/client'
import type { User } from './api/types'

/** ?s=main|chat открывает экран сразу, без входа (удобно при разработке). */
function initialTab(): Tab | null {
  const s = new URLSearchParams(location.search).get('s')
  return s === 'main' || s === 'chat' ? s : null
}

export default function App() {
  const [authed, setAuthed] = useState(() => initialTab() !== null)
  const [tab, setTab] = useState<Tab>(() => initialTab() ?? 'main')
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    let alive = true
    fetchMe().then((u) => alive && setUser(u))
    return () => {
      alive = false
    }
  }, [])

  if (!authed) return <Login onEnter={() => setAuthed(true)} />
  if (!user) return null

  return (
    <Shell tab={tab} onTab={setTab} user={user}>
      {tab === 'main' ? <Home openChat={() => setTab('chat')} /> : <Chat user={user} />}
    </Shell>
  )
}
