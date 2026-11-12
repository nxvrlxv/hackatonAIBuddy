import { Clock, Info, Send, Square } from 'lucide-react'
import { UserAvatar } from './ui'

export function Chat() {
  return (
    <>
      <i className="blob chat-blob-pink" />
      <div className="art hero-art chat-art" aria-hidden="true">
        <i className="blob hero-blob-green" />
        <i className="blob hero-blob-blue" />
        <img className="mascot hero-cat" src="/img/buddy-wave.webp" alt="" />
      </div>

      <h1 className="page-title chat-title">AI-чат</h1>
      <p className="page-sub chat-sub">Задавай вопросы о процессах, инструментах и первой неделе.</p>

      <section className="card chat">
        <div className="msg-user">
          <UserAvatar size={34} />
          <div className="msg-user-body">
            <div className="msg-name"><b>Александр</b><span> · 10:14</span></div>
            <div className="bubble-user">Привет! Как получить доступ к GitLab?</div>
          </div>
        </div>

        <div className="msg-bot">
          <img className="bot-avatar" src="/img/buddy-avatar.webp" alt="" />
          <div className="msg-name bot-name"><b>Buddy</b><span> · 10:15</span></div>
          <div className="bubble-bot">
            <p>Привет, Александр! Помогу по обоим вопросам. Вот<br />короткий порядок действий:</p>
            <div className="act act-blue">
              <span className="act-ico"><Square size={13} fill="#1A8BFF" color="#1A8BFF" /></span>
              <div><b>Доступ к GitLab</b><span>Открой сервис «Доступы», выбери GitLab и отправь заявку.<br />Статус появится в задачах.</span></div>
            </div>
            <div className="act act-pink">
              <span className="act-ico"><Clock size={15} strokeWidth={2.4} color="#F2385C" /></span>
              <div><b>Оформить отгул</b><span>Выбери дату и тип отсутствия, проверь данные и подтверди<br />отправку заявки.</span></div>
            </div>
          </div>
        </div>

        <div className="info">
          <Info size={19} strokeWidth={2} />
          Если ответа нет в базе, Buddy предложит добавить вопрос в повестку встречи с наставником.
        </div>

        <form className="composer" onSubmit={(e) => e.preventDefault()}>
          <input placeholder="Спроси Buddy о работе, процессе или онбордиге..." aria-label="Сообщение" />
          <button type="submit" aria-label="Отправить"><Send size={18} strokeWidth={2.2} /></button>
        </form>
      </section>
    </>
  )
}
