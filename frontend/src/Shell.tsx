import type { ReactNode } from 'react'
import { Icon, Logo, Avatar, Mascot } from './ui'
import type { User } from './api/types'

export type Tab = 'main' | 'chat'

const NAV: { id: Tab; label: string; icon: 'home' | 'chatOutline' }[] = [
  { id: 'main', label: 'Главная', icon: 'home' },
  { id: 'chat', label: 'Чат', icon: 'chatOutline' },
]

export function Shell({
  tab,
  onTab,
  user,
  children,
}: {
  tab: Tab
  onTab: (t: Tab) => void
  user: User
  children: ReactNode
}) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="sb-logo">
          <Logo height={28} />
        </div>

        <nav className="nav">
          {NAV.map((item) => (
            <button
              key={item.id}
              className={`nav-item ${tab === item.id ? 'is-active' : ''}`}
              onClick={() => onTab(item.id)}
              aria-current={tab === item.id ? 'page' : undefined}
            >
              <Icon name={item.icon} size={20} className="nav-ico" />
              {item.label}
            </button>
          ))}
        </nav>

        <button className="sb-user">
          <Avatar size={26} />
          <span className="sb-user-txt">
            <b>{user.short}</b>
            <span>{user.role}</span>
          </span>
          <Icon name="chevronDown" size={13} className="sb-chev" />
        </button>
      </aside>

      <main className="main">
        {/* Арт шапки общий для обеих вкладок и не перерисовывается при их смене:
            котик и круги стоят на одном месте, а меняется только текст и карточки. */}
        <div className="head-art" aria-hidden="true">
          <i className="blob-green" />
          <i className="blob-pink" />
          <i className="blob-blue" />
          <Mascot variant="wave" className="mascot" />
        </div>
        <div className="screen-in" key={tab} style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
          {children}
        </div>
      </main>
    </div>
  )
}
