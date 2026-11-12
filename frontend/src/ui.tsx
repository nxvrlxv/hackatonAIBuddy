import { useLayoutEffect, useState, type ReactNode } from 'react'

/**
 * Холст с размерами исходного макета. Все блоки внутри стоят в тех же координатах,
 * что и в исходнике, а холст целиком масштабируется под окно.
 */
// Отладка: ?ov=1 накладывает исходник (режим разницы) для сверки координат.
const REF: Record<string, { src: string; x: number; y: number; k: number }> = {
  login: { src: '/ref/p1.png', x: 0, y: 0, k: 1 },
  main: { src: '/ref/p3.png', x: 0, y: 0, k: 1 },
  chat: { src: '/ref/p2.png', x: -12.85, y: 25.25, k: 1.15 },
}
const qs = new URLSearchParams(location.search)
const ovMode = qs.get('ov')
const fixed = qs.has('fixed') // отладка: масштаб 1, холст в левом верхнем углу

// Масштаб у всех экранов считается от кадра главной (1086 × 600), чтобы при переходах ничего не прыгало.
const FIT_W = 1086
const FIT_H = 600

export function Stage({ w, h, children, refKey, center }: { w: number; h: number; children: ReactNode; refKey?: string; center?: boolean }) {
  const [zoom, setZoom] = useState(1)
  const [height, setHeight] = useState(h)
  useLayoutEffect(() => {
    const fit = () => {
      const z = fixed ? 1 : Math.min(window.innerWidth / FIT_W, window.innerHeight / FIT_H)
      setZoom(z)
      // по высоте холст дотягивается до низа окна: боковая панель и чат растягиваются, как в исходнике
      setHeight(fixed || center ? h : Math.max(h, Math.floor(window.innerHeight / z)))
    }
    fit()
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [w, h])
  return (
    <div className="viewport" style={fixed ? { placeItems: 'start' } : center ? { placeItems: 'center' } : undefined}>
      <div className="stage" style={{ width: w, height, zoom }}>
        {children}
        {ovMode && refKey && (
          <img
            src={REF[refKey].src}
            alt=""
            style={{
              position: 'absolute', left: REF[refKey].x, top: REF[refKey].y, transformOrigin: '0 0',
              transform: `scale(${REF[refKey].k})`, pointerEvents: 'none', zIndex: 50,
              mixBlendMode: ovMode === 'diff' ? 'difference' : 'normal', opacity: ovMode === 'diff' ? 1 : Number(ovMode) || 0.5,
              inset: 'auto',
            }}
          />
        )}
      </div>
    </div>
  )
}

/** Логотип Avito: четыре круга и надпись. */
/** Знак Avito: пропорции и цвета по официальному логотипу. */
export function AvitoMark({ size = 29 }: { size?: number }) {
  return (
    <svg width={size} height={size * 27 / 29} viewBox="0 0 29 27" aria-hidden="true">
      <circle cx="10.35" cy="5.12" r="3.25" fill="#965EEB" />
      <circle cx="21.68" cy="7.04" r="7.04" fill="#00AAFF" />
      <circle cx="8.69" cy="18.16" r="8.69" fill="#04E061" />
      <circle cx="23.72" cy="20.47" r="5.23" fill="#FF4053" />
    </svg>
  )
}

export function Logo({ size = 29, text = 25 }: { size?: number; text?: number }) {
  return (
    <div className="logo">
      <AvitoMark size={size} />
      <span style={{ fontSize: text }}>Avito</span>
    </div>
  )
}

/** Круглая иконка-аватар пользователя (фиолетовый круг, белый силуэт). */
export function UserAvatar({ size }: { size: number }) {
  return (
    <span className="user-avatar" style={{ width: size, height: size }} aria-hidden="true">
      <svg viewBox="0 0 24 24" width={size * 0.56} height={size * 0.56}>
        <circle cx="12" cy="7.6" r="4.6" fill="#fff" />
        <path d="M3.6 21.2c.6-4.5 4.1-7.4 8.4-7.4s7.8 2.9 8.4 7.4c.1.5-.3.8-.7.8H4.3c-.4 0-.8-.3-.7-.8Z" fill="#fff" />
      </svg>
    </span>
  )
}
