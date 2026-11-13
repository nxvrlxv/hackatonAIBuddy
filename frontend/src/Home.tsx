import { useEffect, useState } from 'react'
import { fetchHome } from './api/client'
import type { HomeData } from './api/types'
import { Icon, Icon3d, Mascot } from './ui'

const PILL: Record<string, { cls: string; label: string; ico: 'check' | 'progress' | 'play' }> = {
  done: { cls: 'pill-done', label: 'Готово', ico: 'check' },
  work: { cls: 'pill-work', label: 'В работе', ico: 'progress' },
  next: { cls: 'pill-next', label: 'Далее', ico: 'play' },
}

const TONE = ['green', 'blue', 'pink'] as const
/* иконки и цвета контактов — по кадру макета p3: фиолетовый аватар, синий календарь, зелёная команда */
const CONTACT_ICON = { mentor: 'user', hr: 'calendarWhite', team: 'users' } as const
const CONTACT_TONE = { mentor: 'violet', hr: 'blue', team: 'green' } as const

export function Home({ openChat }: { openChat: () => void }) {
  const [data, setData] = useState<HomeData | null>(null)

  useEffect(() => {
    let alive = true
    fetchHome().then((d) => alive && setData(d))
    return () => {
      alive = false
    }
  }, [])

  if (!data) return <div className="content" aria-busy="true" />

  const { user, focus, onboarding, contacts } = data
  const pct = Math.round((onboarding.done / onboarding.total) * 100)

  return (
    <>
      <header className="page-head screen-in">
        <h1 className="page-title">Привет, {user.name}!</h1>
        <p className="page-sub">Я Buddy — помогу освоиться, разобраться в процессах и пройти адаптацию.</p>
        <div className="head-art" aria-hidden="true">
          <i className="blob-green" />
          <i className="blob-pink" />
          <i className="blob-blue" />
          <Mascot variant="wave" className="mascot" />
        </div>
      </header>

      <div className="home content">
        <section className="card focus">
          <h2 className="card-title">Твой фокус на сегодня</h2>
          <ul className="focus-list">
            {focus.map((task, i) => {
              const p = PILL[task.status]
              /* иконка отражает суть шага, как в кадре p3: команда, ноутбук, настройки */
              const icon = task.id === 'team' ? 'users' : task.id === 'gitlab' ? 'laptop' : 'settings'
              return (
                <li key={task.id}>
                  <Icon3d tone={TONE[i % TONE.length]}>
                    <Icon name={icon} size={19} />
                  </Icon3d>
                  <span className="focus-text">{task.title}</span>
                  <span className={`pill ${p.cls}`}>
                    {task.status === 'work' ? <i className="ring" /> : <Icon name={p.ico} size={14} className="pill-ico" />}
                    {p.label}
                  </span>
                </li>
              )
            })}
          </ul>
        </section>

        <section className="card onb">
          <h2 className="card-title">Мой онбординг</h2>
          <div className="onb-count">
            <b>
              {onboarding.done}/{onboarding.total}
            </b>
          </div>
          <div className="onb-pct">{pct}%</div>
          <div className="bar">
            <div className="bar-fill" style={{ width: `${pct}%` }} />
          </div>
          <button className="next" onClick={openChat}>
            <span className="next-ico">
              <Icon name="arrowUpRight" size={16} />
            </span>
            <div>
              <b>Следующий шаг</b>
              <span>{onboarding.nextStep}</span>
            </div>
          </button>
        </section>

        <section className="card cont">
          <div className="cont-art" aria-hidden="true">
            <i className="cont-halo" />
            <Mascot variant="cheeks" className="cont-cat mascot" />
          </div>
          <h2 className="card-title">Продолжить с Buddy</h2>
          <p className="cont-text">Задай вопрос, уточни детали или получи подсказку по онбордингу.</p>
          <button className="btn-primary cont-btn" onClick={openChat}>
            <Icon name="chatSolid" size={18} />
            Открыть чат
          </button>
        </section>

        <section className="card contacts">
          <h2 className="card-title">Твои контакты</h2>
          <ul className="contact-list">
            {contacts.map((c) => (
              <li key={c.id}>
                <Icon3d tone={CONTACT_TONE[c.kind]}>
                  <Icon name={CONTACT_ICON[c.kind]} size={c.kind === 'mentor' ? 19 : 18} />
                </Icon3d>
                <b>{c.name}</b>
                <Icon name="chevronRight" size={16} className="chev" />
              </li>
            ))}
          </ul>
        </section>
      </div>
    </>
  )
}
