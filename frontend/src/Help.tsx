import { useEffect } from 'react'
import { Icon } from './ui'

/** Возможности Buddy с примерами: клик по примеру открывает чат и сразу задаёт вопрос. */
const SECTIONS: { title: string; text: string; examples: string[] }[] = [
  {
    title: 'База знаний',
    text: 'Отвечает по внутренним документам и указывает источник.',
    examples: ['Как получить доступ к GitLab?', 'Какие ценности у Авито?'],
  },
  {
    title: 'Адаптация',
    text: 'Показывает чек-лист и отмечает выполненные шаги — прогресс на главной обновится.',
    examples: ['Что мне сделать дальше?', 'Я получил доступ к GitLab'],
  },
  {
    title: 'Команда',
    text: 'Подскажет, кто в команде, кто наставник и к кому обратиться.',
    examples: ['Кто в моей команде?'],
  },
  {
    title: 'Отгулы',
    text: 'Оформит заявку на отгул и покажет статус прошлых заявок.',
    examples: ['Оформи отгул на завтра', 'Какие у меня есть заявки?'],
  },
]

export function Help({ onAsk, onClose }: { onAsk: (question: string) => void; onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="modal-backdrop" onPointerDown={(e) => e.target === e.currentTarget && onClose()}>
      <section className="modal card screen-in" role="dialog" aria-modal="true" aria-labelledby="help-title">
        <div className="modal-head">
          <h2 id="help-title" className="card-title">
            Что умеет Buddy
          </h2>
          <button className="modal-close" onClick={onClose} aria-label="Закрыть">
            ×
          </button>
        </div>
        <p className="modal-sub">Нажми на пример — Buddy сразу ответит в чате.</p>
        <ul className="help-list">
          {SECTIONS.map((s) => (
            <li key={s.title}>
              <b>{s.title}</b>
              <span>{s.text}</span>
              <div className="help-examples">
                {s.examples.map((q) => (
                  <button key={q} className="help-chip" onClick={() => onAsk(q)}>
                    <Icon name="chatOutline" size={14} />
                    {q}
                  </button>
                ))}
              </div>
            </li>
          ))}
        </ul>
        <p className="modal-note">
          Если ответа нет в базе, Buddy честно скажет об этом. Профиль и доступы — в корпоративных системах Авито.
        </p>
      </section>
    </div>
  )
}
