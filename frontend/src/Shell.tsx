import { useEffect, useRef, useState, type ReactNode } from 'react'
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
  onHelp,
  onLogout,
  children,
}: {
  tab: Tab
  onTab: (t: Tab) => void
  user: User
  onHelp: () => void
  onLogout: () => void
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

        <UserMenu user={user} onHelp={onHelp} onLogout={onLogout} />
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
        {children}
      </main>
    </div>
  )
}

/** Кнопка пользователя внизу сайдбара: по клику — меню «Что умеет Buddy» и «Выйти». */
function UserMenu({ user, onHelp, onLogout }: { user: User; onHelp: () => void; onLogout: () => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // закрытие по клику мимо и по Escape
  useEffect(() => {
    if (!open) return
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('pointerdown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const pick = (fn: () => void) => () => {
    setOpen(false)
    fn()
  }

  return (
    <div className="sb-user-wrap" ref={ref}>
      {open && (
        <div className="user-menu" role="menu">
          <div className="user-menu-head">
            <b>{user.short}</b>
            <span>{user.role}</span>
          </div>
          <button role="menuitem" className="user-menu-item" onClick={pick(onHelp)}>
            <Icon name="info" size={16} />
            Что умеет Buddy
          </button>
          <button role="menuitem" className="user-menu-item is-danger" onClick={pick(onLogout)}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <path d="M16 17l5-5-5-5" />
              <path d="M21 12H9" />
            </svg>
            Выйти
          </button>
        </div>
      )}
      <button className={`sb-user${open ? ' is-open' : ''}`} onClick={() => setOpen((v) => !v)} aria-haspopup="menu" aria-expanded={open}>
        <Avatar size={26} />
        <span className="sb-user-txt">
          <b>{user.short}</b>
          <span>{user.role}</span>
        </span>
        <Icon name="chevronDown" size={13} className="sb-chev" />
      </button>
    </div>
  )
}
