import type { ReactNode, SVGProps } from 'react'

/**
 * Иконки лежат отдельными файлами в /public/icons (экспорт из макета).
 * Красим их через CSS-маску: цвет задаётся currentColor, сами SVG остаются как есть —
 * в них зашиты разные цвета заливок, которые для тёмных кнопок не подходят.
 */
const SRC: Record<string, string> = {
  mail: '01-email',
  home: '02-home',
  chatOutline: '03-chat-outline-purple',
  user: '04-user-white',
  service: '05-service-square-blue',
  clock: '06-clock-red',
  info: '07-info',
  send: '08-send-paper-plane-white',
  users: '09-users-white',
  laptop: '10-laptop-white',
  settings: '11-settings-white',
  calendarWhite: '12-hr-calendar-white',
  arrowUpRight: '13-arrow-up-right-white',
  check: '14-check-green',
  progress: '15-progress-yellow',
  play: '16-play-dark',
  chevronRight: '17-chevron-right',
  chevronDown: '18-chevron-down',
  chatSolid: '19-chat-solid-white',
  calendarBlue: '20-hr-calendar-blue',
}

export type IconName = keyof typeof SRC

export function Icon({ name, size = 18, ...rest }: { name: IconName; size?: number } & Omit<SVGProps<HTMLSpanElement>, 'name'>) {
  const url = `url("/icons/${SRC[name]}.svg")`
  return (
    <span
      aria-hidden="true"
      {...rest}
      style={{
        display: 'inline-block',
        flex: 'none',
        width: size,
        height: size,
        backgroundColor: 'currentColor',
        maskImage: url,
        WebkitMaskImage: url,
        maskSize: 'contain',
        WebkitMaskSize: 'contain',
        maskRepeat: 'no-repeat',
        WebkitMaskRepeat: 'no-repeat',
        maskPosition: 'center',
        WebkitMaskPosition: 'center',
        ...rest.style,
      }}
    />
  )
}

/** Знак Avito — четыре круга, файл из img. */
export function AvitoMark({ size = 28 }: { size?: number }) {
  return <img src="/icons/avito-mark.svg" width={size} height={size * (470 / 570)} alt="" aria-hidden="true" />
}

/** Полный логотип Avito (картинка из img). */
export function Logo({ height = 26 }: { height?: number }) {
  return <img className="logo-img" src="/img/avito-logo.png" height={height} alt="Avito" style={{ height }} />
}

export function Mascot({ variant, alt = '', className, style }: { variant: 'wave' | 'cheeks' | 'avatar'; alt?: string; className?: string; style?: React.CSSProperties }) {
  return <img className={className} src={`/img/buddy-${variant}.webp`} alt={alt} style={style} draggable={false} />
}

export function Avatar({ size = 34 }: { size?: number }) {
  return (
    <span className="avatar" style={{ width: size, height: size }} aria-hidden="true">
      <Icon name="user" size={size * 0.56} />
    </span>
  )
}

export function Icon3d({ tone, children }: { tone: 'green' | 'blue' | 'pink' | 'violet'; children: ReactNode }) {
  return <span className={`ico ico-${tone}`}>{children}</span>
}
