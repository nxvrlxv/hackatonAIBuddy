import { ArrowUpRight, CalendarHeart, CaretRight, Check, GearSix, Laptop, Play, UsersThree } from '@phosphor-icons/react'
import { MessageCircle } from 'lucide-react'
import { UserAvatar } from './ui'

export function Home({ openChat }: { openChat: () => void }) {
  return (
    <>
      <i className="blob main-blob-pink" />
      <div className="art hero-art" aria-hidden="true">
        <i className="blob hero-blob-green" />
        <i className="blob hero-blob-blue" />
        <img className="mascot hero-cat" src="/img/buddy-wave.webp" alt="" />
      </div>

      <h1 className="page-title main-title">Привет, Александр!</h1>
      <p className="page-sub main-sub">Я Buddy — помогу освоиться, разобраться<br />в процессах и пройти адаптацию.</p>

      <section className="card focus">
        <h3 className="card-title">Твой фокус на сегодня</h3>
        <ul className="focus-list">
          <li>
            <span className="ico ico-green"><UsersThree size={21} weight="fill" color="#fff" /></span>
            <span className="focus-text">Познакомиться с командой</span>
            <span className="pill pill-done"><Check size={15} weight="bold" color="#0FB52F" />Готово</span>
          </li>
          <li>
            <span className="ico ico-blue"><Laptop size={20} weight="fill" color="#fff" /></span>
            <span className="focus-text">Получить доступ к GitLab</span>
            <span className="pill pill-work"><i className="ring" />В работе</span>
          </li>
          <li>
            <span className="ico ico-pink"><GearSix size={20} weight="fill" color="#fff" /></span>
            <span className="focus-text">Настроить рабочее окружение</span>
            <span className="pill pill-next"><Play size={13} weight="fill" color="#15172B" />Далее</span>
          </li>
        </ul>
      </section>

      <section className="card cont">
        <div className="art cont-art" aria-hidden="true">
          <i className="blob cont-halo" />
          <img className="mascot cont-cat" src="/img/buddy-cheeks.webp" alt="" />
        </div>
        <h3 className="card-title">Продолжить с Buddy</h3>
        <p className="cont-text">Задай вопрос, уточни детали<br />или получи подсказку<br />по онбордингу.</p>
        <button className="btn-primary cont-btn" onClick={openChat}>
          <MessageCircle size={19} strokeWidth={2.1} />
          Открыть чат
        </button>
      </section>

      <section className="card onb">
        <h3 className="card-title">Мой онбординг</h3>
        <div className="onb-count">4/7</div>
        <div className="onb-pct">57%</div>
        <div className="bar"><div className="bar-fill" /></div>
        <div className="next">
          <span className="next-ico"><ArrowUpRight size={17} weight="bold" color="#fff" /></span>
          <div><b>Следующий шаг</b><span>Получить доступ к GitLab</span></div>
        </div>
      </section>

      <section className="card contacts">
        <h3 className="card-title">Твои контакты</h3>
        <ul className="contact-list">
          <li><UserAvatar size={34} /><b>Наставник</b><CaretRight size={16} weight="bold" className="chev" /></li>
          <li>
            <span className="ico ico-blue"><CalendarHeart size={20} weight="fill" color="#fff" /></span>
            <b>HR BP</b><CaretRight size={16} weight="bold" className="chev" />
          </li>
          <li>
            <span className="ico ico-green"><UsersThree size={21} weight="fill" color="#fff" /></span>
            <b>Команда</b><CaretRight size={16} weight="bold" className="chev" />
          </li>
        </ul>
      </section>
    </>
  )
}
