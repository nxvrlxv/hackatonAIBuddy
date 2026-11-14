export type Role = 'user' | 'bot'

export interface ChatMessage {
  id: string
  role: Role
  text: string
  at: string
}

export interface User {
  /** Имя для приветствия: «Саша». */
  name: string
  /** Короткая подпись в боковой панели: «Саша И.». */
  short: string
  /** Роль по-русски: «Стажёр», «Наставник». */
  role: string
}

export type TaskStatus = 'done' | 'work' | 'next'

export interface FocusTask {
  id: string
  title: string
  status: TaskStatus
}

export interface Onboarding {
  done: number
  total: number
  nextStep: string
}

export type ContactKind = 'mentor' | 'hr' | 'team'

export interface Contact {
  id: string
  name: string
  kind: ContactKind
}

export interface HomeData {
  user: User
  focus: FocusTask[]
  onboarding: Onboarding
  contacts: Contact[]
}

/** Колбэки потокового ответа Buddy. */
export interface StreamHandlers {
  /** Очередной кусок текста ответа (уже склеенный целиком: весь текст на данный момент). */
  onText?: (fullText: string) => void
  /** Служебный статус агента, например «Восстанавливаю контекст». */
  onStatus?: (text: string) => void
  /** Внешняя отмена (уход со страницы, «Начать заново»). */
  signal?: AbortSignal
}
