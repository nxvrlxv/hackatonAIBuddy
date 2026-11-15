import { useEffect, useRef, useState } from 'react'
import { ago, ApiError, approveTask, fetchReviews, REVIEW_COMMENT_MAX, rejectTask, reviewErrorText } from './api/client'
import type { ReviewRequest } from './api/types'
import { Icon, Icon3d } from './ui'

/**
 * «Запросы на проверку» — первая карточка главной у наставника.
 * Принять — сразу; отклонить — инлайн-форма с обязательным комментарием (он уйдёт стажёру и Buddy).
 * active — вкладка видна: при возврате на главную список перечитывается.
 * onResolved — запрос принят или отклонён: главная обновляет прогресс стажёров.
 */
export function Reviews({ active, onResolved }: { active: boolean; onResolved?: () => void }) {
  const [items, setItems] = useState<ReviewRequest[] | null>(null)
  const [failed, setFailed] = useState(false)
  /** задача, по которой идёт запрос */
  const [busy, setBusy] = useState<string | null>(null)
  /** задача, для которой открыта форма отклонения */
  const [rejecting, setRejecting] = useState<string | null>(null)
  const [comment, setComment] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  /** строка над списком: запрос пропал, пока наставник смотрел (404/409) */
  const [notice, setNotice] = useState('')

  useEffect(() => {
    if (!active) return
    let alive = true
    fetchReviews()
      .then((r) => {
        if (!alive) return
        setItems(r)
        setFailed(false)
      })
      .catch(() => alive && setFailed(true))
    return () => {
      alive = false
    }
  }, [active])

  const drop = (id: string) => setItems((list) => list && list.filter((r) => r.taskId !== id))

  const act = async (r: ReviewRequest, run: () => Promise<void>, done: string) => {
    setBusy(r.taskId)
    setErrors((m) => {
      const next = { ...m }
      delete next[r.taskId]
      return next
    })
    setNotice('')
    try {
      await run()
      drop(r.taskId)
      setRejecting(null)
      setComment('')
      setNotice(done)
      onResolved?.()
    } catch (e) {
      // запроса больше нет (другой наставник, повторный клик) — убираем его из списка
      if (e instanceof ApiError && (e.status === 404 || e.status === 409)) {
        drop(r.taskId)
        setRejecting(null)
        setNotice(`«${r.taskTitle}»: ${reviewErrorText(e)}`)
        onResolved?.()
      } else setErrors((m) => ({ ...m, [r.taskId]: reviewErrorText(e) }))
    } finally {
      setBusy(null)
    }
  }

  const openReject = (id: string) => {
    setRejecting(id)
    setComment('')
  }

  const count = items?.length ?? 0

  return (
    <>
      <h2 className="card-title reviews-title">
        Запросы на проверку
        {count > 0 && <span className="reviews-count">{count}</span>}
      </h2>
      {notice && (
        <p className="reviews-notice" role="status">
          {notice}
        </p>
      )}
      {items === null ? (
        failed ? (
          <p className="reviews-empty is-error">Не получилось загрузить запросы. Обнови страницу.</p>
        ) : (
          <p className="reviews-empty" aria-busy="true">
            Загружаю…
          </p>
        )
      ) : count === 0 ? (
        <p className="reviews-empty">Новых запросов нет. Когда стажёр отправит задачу, она появится здесь.</p>
      ) : (
        <ul className="reviews-list">
          {items.map((r) => {
            const isBusy = busy === r.taskId
            return (
              <li key={r.taskId} className={`review${rejecting === r.taskId ? ' is-open' : ''}`}>
                <div className="review-main">
                  <Icon3d tone="violet">
                    <Icon name="user" size={17} />
                  </Icon3d>
                  <div className="review-text">
                    <span className="review-who">
                      <b title={r.internName}>{r.internName}</b>
                      <time dateTime={r.submittedAt} title={new Date(r.submittedAt).toLocaleString('ru-RU')}>
                        {ago(r.submittedAt)}
                      </time>
                    </span>
                    <span className="review-task" title={r.taskDescription || r.taskTitle}>
                      {r.taskTitle}
                      {r.stageTitle && <i> · {r.stageTitle}</i>}
                    </span>
                  </div>
                  <div className="review-actions">
                    <button
                      className="review-btn is-approve"
                      disabled={busy !== null}
                      aria-busy={isBusy && rejecting !== r.taskId}
                      onClick={() => act(r, () => approveTask(r.taskId), `Принято: «${r.taskTitle}» — ${r.internName}.`)}
                    >
                      <Icon name="check" size={13} />
                      Принять
                    </button>
                    <button
                      className="review-btn"
                      disabled={busy !== null}
                      aria-expanded={rejecting === r.taskId}
                      onClick={() => (rejecting === r.taskId ? setRejecting(null) : openReject(r.taskId))}
                    >
                      Отклонить
                    </button>
                  </div>
                </div>
                {rejecting === r.taskId && (
                  <RejectForm
                    value={comment}
                    onChange={setComment}
                    busy={isBusy}
                    onCancel={() => setRejecting(null)}
                    onSubmit={() =>
                      act(r, () => rejectTask(r.taskId, comment), `Вернули на доработку: «${r.taskTitle}» — ${r.internName}.`)
                    }
                  />
                )}
                {errors[r.taskId] && (
                  <p className="review-error" role="alert">
                    {errors[r.taskId]}
                  </p>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}

/** Комментарий обязателен: кнопка неактивна, пока поле пустое; лимит как на бэкенде. */
function RejectForm({
  value,
  onChange,
  busy,
  onCancel,
  onSubmit,
}: {
  value: string
  onChange: (v: string) => void
  busy: boolean
  onCancel: () => void
  onSubmit: () => void
}) {
  const ref = useRef<HTMLTextAreaElement>(null)
  useEffect(() => ref.current?.focus(), [])
  const empty = value.trim() === ''

  return (
    <form
      className="review-reject"
      onSubmit={(e) => {
        e.preventDefault()
        if (!empty && !busy) onSubmit()
      }}
    >
      <textarea
        ref={ref}
        value={value}
        maxLength={REVIEW_COMMENT_MAX}
        rows={2}
        placeholder="Что доработать? Комментарий увидят стажёр и Buddy"
        aria-label="Комментарий для стажёра"
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onCancel()
          // Ctrl/⌘+Enter — отправить
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) e.currentTarget.form?.requestSubmit()
        }}
      />
      <div className="review-reject-bar">
        <span className="review-count">
          {value.length}/{REVIEW_COMMENT_MAX}
        </span>
        <button type="button" className="review-btn is-ghost" onClick={onCancel} disabled={busy}>
          Отмена
        </button>
        <button type="submit" className="review-btn is-reject" disabled={empty || busy} aria-busy={busy}>
          Вернуть на доработку
        </button>
      </div>
    </form>
  )
}
