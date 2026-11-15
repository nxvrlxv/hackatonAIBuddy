import { useEffect, useState } from 'react'
import { fetchInterns } from './api/client'
import type { InternProgress } from './api/types'

/** Сколько стажёров помещается в карточку; остальные — строкой «и ещё N». */
const ROWS = 3

/**
 * «Мои стажёры» — карточка онбординга у наставника: прогресс адаптации каждого стажёра.
 * active — вкладка видна; refresh растёт после принятия/отклонения задачи — прогресс перечитывается.
 */
export function Interns({ active, refresh }: { active: boolean; refresh: number }) {
  const [items, setItems] = useState<InternProgress[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!active) return
    let alive = true
    fetchInterns()
      .then((list) => {
        if (!alive) return
        setItems(list)
        setFailed(false)
      })
      .catch(() => alive && setFailed(true))
    return () => {
      alive = false
    }
  }, [active, refresh])

  return (
    <>
      <h2 className="card-title">Мои стажёры</h2>
      {items === null ? (
        failed ? (
          <p className="interns-empty is-error">Не получилось загрузить стажёров. Обнови страницу.</p>
        ) : (
          <p className="interns-empty" aria-busy="true">
            Загружаю…
          </p>
        )
      ) : items.length === 0 ? (
        <p className="interns-empty">За тобой пока не закреплены стажёры. Когда появятся — здесь будет их прогресс адаптации.</p>
      ) : (
        <>
          <ul className="interns-list">
            {items.slice(0, ROWS).map((i) => (
              <InternRow key={i.id} intern={i} />
            ))}
          </ul>
          {items.length > ROWS && <p className="interns-more">и ещё {items.length - ROWS}</p>}
        </>
      )}
    </>
  )
}

function InternRow({ intern: i }: { intern: InternProgress }) {
  const finished = i.total > 0 && i.done >= i.total
  return (
    <li className={`intern${finished ? ' is-done' : ''}`}>
      <div className="intern-head">
        <b title={i.name}>{i.name}</b>
        <span className="intern-count">
          {i.done}/{i.total} · {i.percent}%
        </span>
      </div>
      <div
        className="intern-bar"
        role="progressbar"
        aria-label={`Адаптация: ${i.name}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={i.percent}
      >
        <div className="intern-bar-fill" style={{ width: `${i.percent}%` }} />
      </div>
      <div className="intern-meta">
        <span className="intern-stage" title={i.stage || undefined}>
          {finished ? 'Адаптация пройдена' : i.stage || 'Ещё не начал'}
        </span>
        {i.inReview > 0 && <span className="intern-review">{i.inReview} на проверке</span>}
      </div>
    </li>
  )
}
