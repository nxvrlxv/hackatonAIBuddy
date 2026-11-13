export type Role = 'user' | 'bot'

export interface ChatMessage {
  id: string
  role: Role
  text: string
  at: string
}

export interface User {
  name: string
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
