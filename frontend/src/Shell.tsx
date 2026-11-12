import { ChevronDown, MessageCircle } from 'lucide-react'
import type { ReactNode } from 'react'
import { Logo, Stage, UserAvatar } from './ui'

export type Tab = 'main' | 'chat'

export function Shell({ tab, onTab, children }: { tab: Tab; onTab: (t: Tab) => void; children: ReactNode }) {
  return (
    <Stage w={1086} h={600} refKey={tab}>
      <div className="app">
        <aside className="sidebar">
          <div className="sb-logo"><Logo /></div>
          <nav>
            <button className={`nav nav-1 ${tab === 'main' ? 'is-active' : ''}`} onClick={() => onTab('main')}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinejoin="round" aria-hidden="true"><path d="M3.5 10.2 12 3.2l8.5 7V20a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1Z" /></svg>
              Главная
            </button>
            <button className={`nav nav-2 ${tab === 'chat' ? 'is-active' : ''}`} onClick={() => onTab('chat')}>
              <MessageCircle size={18} strokeWidth={2} />
              AI-чат
            </button>
          </nav>
          <div className="user">
            <UserAvatar size={26} />
            <div className="user-txt"><b>Александр С.</b><span>Стажёр</span></div>
            <ChevronDown size={15} strokeWidth={2.4} className="user-chev" />
          </div>
        </aside>
        <div className="screen screen-in" key={tab}>{children}</div>
      </div>
    </Stage>
  )
}
