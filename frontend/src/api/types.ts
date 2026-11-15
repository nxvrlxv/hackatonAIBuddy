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
  /** Машинная роль: по ней главная решает, что показывать; неизвестная роль — 'other'. */
  kind: UserKind
}

export type UserKind = 'intern' | 'mentor' | 'lead' | 'other'

/** done — принята, work — в работе (todo→in_progress), review — на проверке у наставника, next — не начата. */
export type TaskStatus = 'done' | 'work' | 'review' | 'next'

export interface FocusTask {
  id: string
  title: string
  status: TaskStatus
  /** Когда отправлена на проверку (ISO), есть только у review. */
  submittedAt?: string
  /** Комментарий наставника при отклонении: есть только у work — «Нужно доработать». */
  reviewComment?: string
}

/** Запрос на проверку задачи стажёра — для наставника. */
export interface ReviewRequest {
  taskId: string
  taskTitle: string
  taskDescription: string
  stageTitle: string
  internId: string
  internName: string
  /** ISO-время отправки */
  submittedAt: string
}

/** Прогресс адаптации стажёра — для карточки наставника «Мои стажёры». */
export interface InternProgress {
  id: string
  name: string
  done: number
  total: number
  percent: number
  /** сколько задач ждут проверки наставника */
  inReview: number
  /** текущая стадия; '' — все задачи приняты */
  stage: string
}

export interface Onboarding {
  done: number
  total: number
  nextStep: string
}

export type TeamRole = 'lead' | 'mentor' | 'intern'

export interface TeamMember {
  id: string
  name: string
  /** lead | mentor | intern; неизвестная роль приходит как есть */
  role: TeamRole | string
  /** личный наставник пользователя */
  isMyMentor: boolean
}

export interface HomeData {
  user: User
  focus: FocusTask[]
  onboarding: Onboarding
  /** коллеги по команде: сначала свой наставник, затем лиды, наставники, стажёры */
  team: TeamMember[]
  /** название команды */
  teamName: string
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
