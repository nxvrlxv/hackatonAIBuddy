import { useState } from 'react'
import { Chat } from './Chat'
import { Login } from './Login'
import { Home } from './Home'
import { Shell, type Tab } from './Shell'

export default function App() {
  const [screen, setScreen] = useState<'login' | Tab>(() => (new URLSearchParams(location.search).get('s') as Tab) || 'login')
  if (screen === 'login') return <Login onEnter={() => setScreen('main')} />
  return (
    <Shell tab={screen} onTab={setScreen}>
      {screen === 'main' ? <Home openChat={() => setScreen('chat')} /> : <Chat />}
    </Shell>
  )
}
