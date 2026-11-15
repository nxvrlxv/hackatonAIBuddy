import { useEffect, useState } from 'react'
import { ago, fetchHome, reviewErrorText, submitForReview } from './api/client'
import type { FocusTask, HomeData, User } from './api/types'
import { Interns } from './Interns'
import { Reviews } from './Reviews'
import { Icon, Icon3d, Mascot } from './ui'

const PILL: Record<string, { cls: string; label: string; ico: 'check' | 'progress' | 'play' | 'clock' | 'info' }> = {
  done: { cls: 'pill-done', label: 'Готово', ico: 'check' },
  work: { cls: 'pill-work', label: 'В работе', ico: 'progress' },
  review: { cls: 'pill-review', label: 'На проверке', ico: 'clock' },
  rework: { cls: 'pill-rework', label: 'Нужно доработать', ico: 'info' },
  next: { cls: 'pill-next', label: 'Далее', ico: 'play' },
}
/** Плашка строки: возвращённая с комментарием задача — «Нужно доработать». */
const pillOf = (t: FocusTask) => PILL[t.status === 'work' && t.reviewComment ? 'rework' : t.status]
/** Отправить на проверку можно не начатую и начатую (в т.ч. возвращённую) задачу. */
const canSubmit = (t: FocusTask) => t.status === 'next' || t.status === 'work'

const TONE = ['green', 'blue', 'pink'] as const
/* иконки задач по порядку, как в кадре p3: команда, ноутбук, настройки */
const TASK_ICON = ['users', 'laptop', 'settings'] as const
/* цвет кружка по роли коллеги — палитра кадра p3 */
const ROLE_TONE: Record<string, 'violet' | 'blue' | 'green'> = { mentor: 'violet', lead: 'blue', intern: 'green' }
const ROLE_LABEL: Record<string, string> = { lead: 'Руководитель', mentor: 'Наставник', intern: 'Стажёр' }
/** Сколько коллег помещается в карточку; остальные — строкой «и ещё N». */
const TEAM_ROWS = 3

/**
 * active — вкладка видна. Главная не размонтируется при уходе в чат,
 * поэтому при каждом возврате данные перечитываются: Buddy мог отметить задачу.
 */
/** Заготовка вопроса про задачу: подставляется в поле ввода чата, пользователь может дописать. */
const askAbout = (title: string) => `Как выполнить задачу «${title}»?`

/**
 * openChat(draft) — открыть чат; draft, если передан, встаёт в поле ввода (не отправляется).
 */
export function Home({ user, active, openChat }: { user: User; active: boolean; openChat: (draft?: string) => void }) {
  const [data, setData] = useState<Omit<HomeData, 'user'> | null>(null)
  const [failed, setFailed] = useState(false)
  /** Растёт после отправки на проверку — главная перечитывается без перезагрузки. */
  const [version, setVersion] = useState(0)
  const [sending, setSending] = useState<string | null>(null)
  const [sendError, setSendError] = useState('')
  /** Растёт, когда наставник принял/отклонил задачу — «Мои стажёры» перечитывают прогресс. */
  const [resolved, setResolved] = useState(0)

  useEffect(() => {
    if (!active) return
    let alive = true
    fetchHome()
      .then((d) => {
        if (!alive) return
        setData(d)
        setFailed(false)
      })
      .catch(() => alive && setFailed(true))
    return () => {
      alive = false
    }
  }, [active, version])

  const submit = async (task: FocusTask) => {
    setSending(task.id)
    setSendError('')
    try {
      await submitForReview(task.id)
      // сразу показываем «На проверке», не дожидаясь перечитывания
      setData((d) => d && { ...d, focus: d.focus.map((t) => (t.id === task.id ? { id: t.id, title: t.title, status: 'review' } : t)) })
    } catch (e) {
      setSendError(reviewErrorText(e))
    } finally {
      setSending(null)
      setVersion((v) => v + 1)
    }
  }

  if (!data)
    return (
      <div className="content" aria-busy={!failed}>
        {failed && <p className="home-error">Не получилось загрузить данные. Обнови страницу.</p>}
      </div>
    )

  const { focus, onboarding, team, teamName } = data
  const pct = onboarding.total ? Math.round((onboarding.done / onboarding.total) * 100) : 0

  return (
    <>
      <header className="page-head">
        <h1 className="page-title">Привет, {user.name}!</h1>
        <p className="page-sub">Я Buddy - помогу освоиться, разобраться<br />в процессах и пройти адаптацию.</p>
      </header>

      <div className="home content">
        <section className="card focus">
          {/* у наставника первая карточка — запросы стажёров на проверку */}
          {user.kind === 'mentor' ? (
            <Reviews active={active} onResolved={() => setResolved((n) => n + 1)} />
          ) : (
            <>
              <h2 className="card-title">Текущие задачи</h2>
              {/* чек-лист адаптации есть только у стажёров; у лида — пояснение вместо пустоты */}
              {onboarding.total === 0 && (
                <p className="home-hint">
                  Чек-лист адаптации ведут стажёры. Buddy подскажет по процессам, документам и составу команды.
                </p>
              )}
            </>
          )}
          {focus.length > 0 && (
            <ul className="focus-list">
              {focus.map((task, i) => {
                const p = pillOf(task)
                const icon = TASK_ICON[i % TASK_ICON.length]
                const submittable = canSubmit(task)
                return (
                  <li key={task.id} className={`${task.reviewComment ? 'has-note' : ''}${submittable ? ' has-submit' : ''}`}>
                    <button className="focus-row" onClick={() => openChat(askAbout(task.title))} title="Спросить Buddy про эту задачу">
                      <Icon3d tone={TONE[i % TONE.length]}>
                        <Icon name={icon} size={19} />
                      </Icon3d>
                      <span className="focus-text">
                        <span className="focus-title">{task.title}</span>
                        {task.reviewComment && (
                          <span className="focus-note" title={task.reviewComment}>
                            Наставник: {task.reviewComment}
                          </span>
                        )}
                      </span>
                      {/* «Далее» не показываем: это и так видно по списку; оставляем только начатые и готовые */}
                      {task.status !== 'next' && (
                        <span
                          className={`pill ${p.cls}`}
                          title={task.submittedAt ? `Отправлено наставнику ${ago(task.submittedAt)}` : p.label}
                        >
                          {p === PILL.work ? <i className="ring" /> : <Icon name={p.ico} size={14} className="pill-ico" />}
                          <span className="pill-label">{p.label}</span>
                        </span>
                      )}
                      {/* место под кнопку «Отправить на проверку»: вложить кнопку в кнопку нельзя */}
                      {submittable && <span className="focus-slot" />}
                      <span className="focus-go">
                        <Icon name="arrowUpRight" size={14} />
                      </span>
                    </button>
                    {submittable && (
                      <button
                        className="focus-submit"
                        onClick={(e) => {
                          e.stopPropagation()
                          void submit(task)
                        }}
                        disabled={sending !== null}
                        aria-busy={sending === task.id}
                        aria-label={`Отправить на проверку: ${task.title}`}
                        title="Отправить наставнику на проверку"
                      >
                        <Icon name="check" size={14} />
                      </button>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
          {sendError && (
            <p className="focus-error" role="alert">
              {sendError}
            </p>
          )}
        </section>

        {/* у наставника вместо своего (пустого) онбординга — прогресс его стажёров */}
        {user.kind === 'mentor' ? (
          <section className="card onb interns">
            <Interns active={active} refresh={resolved} />
          </section>
        ) : (
          <section className="card onb">
            <h2 className="card-title">Мой онбординг</h2>
            {onboarding.total === 0 ? (
              <p className="home-hint">Адаптация тебе не назначена — ты уже в команде.</p>
            ) : (
              <>
                <div className="onb-count">
                  <b>
                    {onboarding.done}/{onboarding.total}
                  </b>
                </div>
                <div className="onb-pct">{pct}%</div>
                <div className="bar">
                  <div className="bar-fill" style={{ width: `${pct}%` }} />
                </div>
              </>
            )}
            <button
              className="next"
              onClick={() => openChat(onboarding.total > onboarding.done ? askAbout(onboarding.nextStep) : undefined)}
            >
              <span className="next-ico">
                <Icon name="arrowUpRight" size={16} />
              </span>
              <div>
                <b>{onboarding.total === 0 ? 'Спросить Buddy' : 'Следующий шаг'}</b>
                <span>{onboarding.total === 0 ? 'Процессы, документы, команда' : onboarding.nextStep}</span>
              </div>
            </button>
          </section>
        )}

        <section className="card cont">
          <div className="cont-art" aria-hidden="true">
            <i className="cont-halo" />
            <Mascot variant="cheeks" className="cont-cat mascot" />
          </div>
          <h2 className="card-title">Продолжить с Buddy</h2>
          <p className="cont-text">Задай вопрос, уточни детали <br />или получи подсказку <br />по онбордингу.</p>
          <button className="btn-primary cont-btn" onClick={() => openChat()}>
            <Icon name="chatSolid" size={18} />
            Открыть чат
          </button>
        </section>

        <section className="card contacts">
          <h2 className="card-title">Твоя команда</h2>
          <p className="team-name">{teamName}</p>
          {team.length === 0 ? (
            <p className="team-empty">Пока в команде только ты.</p>
          ) : (
            <ul className="contact-list">
              {team.slice(0, TEAM_ROWS).map((m) => (
                <li key={m.id}>
                  <Icon3d tone={ROLE_TONE[m.role] ?? 'green'}>
                    <Icon name="user" size={19} />
                  </Icon3d>
                  <b title={m.name}>{m.name}</b>
                  <span className={`team-role${m.isMyMentor ? ' is-mine' : ''}`}>
                    {m.isMyMentor ? 'Твой наставник' : (ROLE_LABEL[m.role] ?? m.role)}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {team.length > TEAM_ROWS && <p className="team-more">и ещё {team.length - TEAM_ROWS} — спроси Buddy «кто в моей команде?»</p>}
        </section>
      </div>
    </>
  )
}
