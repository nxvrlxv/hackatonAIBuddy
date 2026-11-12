import { Mail } from 'lucide-react'
import { AvitoMark, Logo, Stage } from './ui'

export function Login({ onEnter }: { onEnter: () => void }) {
  return (
    <Stage w={725} h={506} refKey="login" center>
      <div className="login screen-in">
        <i className="blob login-blob-green" />
        <div className="login-logo"><Logo size={30} text={25} /></div>

        <section className="lcard lcard-left">
          <span className="badge">AVITO START</span>
          <h1 className="login-title">Добро пожаловать<br />в AI Buddy</h1>
          <p className="login-sub">Я Buddy — помогу освоиться, разобраться<br />в процессах и пройти адаптацию.</p>

          <button className="lbtn lbtn-1" onClick={onEnter}>
            <AvitoMark size={29} />
            Войти через Avito ID
          </button>

          <div className="or"><span>или</span></div>

          <button className="lbtn lbtn-2" onClick={onEnter}>
            <Mail size={21} strokeWidth={2.1} />
            Войти по email
          </button>

          <p className="login-note">Продолжая, ты входишь в тестовый прототип AI Buddy.</p>
        </section>

        <section className="lcard lcard-right">
          <div className="art login-art" aria-hidden="true">
            <i className="blob login-blob-lav" />
            <i className="blob login-blob-blue" />
            <img className="mascot login-cat" src="/img/buddy-wave.webp" alt="" />
          </div>
          <h2 className="login-h2">Твой помощник на первой неделе</h2>
          <p className="login-p">Подскажет следующий шаг, найдёт нужную<br />инструкцию и поможет оформить простую заявку.</p>
        </section>
      </div>
    </Stage>
  )
}
