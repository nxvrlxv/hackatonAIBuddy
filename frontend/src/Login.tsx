import { useState, type CSSProperties } from 'react'
import { errorText, login } from './api/client'
import type { User } from './api/types'
import { useLoginScale } from './loginScale'
import { AvitoMark, Icon, Logo, Mascot } from './ui'

export function Login({ onEnter }: { onEnter: (u: User) => void }) {
  const k = useLoginScale()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function enter() {
    setBusy(true)
    setError('')
    try {
      onEnter(await login())
    } catch (e) {
      setError(errorText(e, 'Не получилось войти. Попробуй ещё раз.'))
      setBusy(false)
    }
  }

  return (
    <div className="login-wrap" style={{ '--k': k } as CSSProperties}>
      <div className="login-brand">
        <Logo height={28} />
      </div>
      <div className="login-frame">
        {/* четыре круга в цветах логотипа: зелёный у верхнего края, розовый за правой карточкой,
            голубой за котиком, красный за левой карточкой снизу */}
        <i className="blob-green" aria-hidden="true" />
        <i className="blob-pink" aria-hidden="true" />
        <i className="blob-red" aria-hidden="true" />
        <div className="login">
          <section className="login-card screen-in">
            <span className="login-badge">AVITO START</span>
            <h1 className="login-title">
              Добро пожаловать
              <br />в AI Buddy
            </h1>
            <p className="login-sub">Я Buddy - помогу освоиться, разобраться<br />в процессах и пройти адаптацию.</p>

            <div className="login-actions">
              <button className="lbtn" onClick={enter} disabled={busy}>
                <AvitoMark size={30} />
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

            {error && <p className="login-error" role="alert">{error}</p>}
            <p className="login-note">Продолжая, ты входишь в тестовый прототип AI Buddy.</p>
          </section>

          <section className="login-art screen-in" aria-hidden="true">
            <div className="login-stage">
              <i className="blob-blue" />
              <Mascot variant="wave" className="cat mascot" />
            </div>
            <h2>Твой помощник на первой неделе</h2>
            <p>Подскажет следующий шаг, найдёт нужную<br />инструкцию и поможет оформить простую заявку.</p>
          </section>
        </div>
      </div>
    </div>
  )
}
