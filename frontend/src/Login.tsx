import { useState } from 'react'
import { login } from './api/client'
import { AvitoMark, Icon, Logo, Mascot } from './ui'

export function Login({ onEnter }: { onEnter: () => void }) {
  const [busy, setBusy] = useState(false)

  async function enter() {
    setBusy(true)
    await login()
    onEnter()
  }

  return (
    <div className="login-wrap">
      <i className="blob-green" aria-hidden="true" />
      <div className="login-brand">
        <Logo height={26} />
      </div>

      <div className="login">
        <section className="login-card screen-in">
          <span className="login-badge">AVITO START</span>
          <h1 className="login-title">
            Добро пожаловать
            <br />в AI Buddy
          </h1>
          <p className="login-sub">Я Buddy — помогу освоиться, разобраться в процессах и пройти адаптацию.</p>

          <div className="login-actions">
            <button className="lbtn" onClick={enter} disabled={busy}>
              <AvitoMark size={26} />
              Войти через Avito ID
            </button>

            <div className="or">
              <span>или</span>
            </div>

            <button className="lbtn" onClick={enter} disabled={busy}>
              <Icon name="mail" size={22} />
              Войти по email
            </button>
          </div>

          <p className="login-note">Продолжая, ты входишь в тестовый прототип AI Buddy.</p>
        </section>

        <section className="login-art screen-in" aria-hidden="true">
          <div className="login-stage">
            <i className="blob-1" />
            <i className="blob-2" />
            <Mascot variant="wave" className="cat mascot" />
          </div>
          <h2>Твой помощник на первой неделе</h2>
          <p>Подскажет следующий шаг, найдёт нужную инструкцию и поможет оформить простую заявку.</p>
        </section>
      </div>
    </div>
  )
}
