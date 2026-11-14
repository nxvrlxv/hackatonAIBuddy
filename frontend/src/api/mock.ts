import type { HomeData, User } from './types'

/**
 * Временные данные вместо ответов API.
 * Пользователь и чат уже берутся из бэкенда (режим api); онбординг и контакты — пока здесь,
 * у бэкенда для них ещё нет ручек.
 */
export const MOCK_USER: User = { name: 'Александр', short: 'Александр С.', role: 'Стажёр' }

export const HOME: Omit<HomeData, 'user'> = {
  focus: [
    { id: 'team', title: 'Познакомиться с командой', status: 'next' },
    { id: 'gitlab', title: 'Получить доступ к GitLab', status: 'next' },
    { id: 'env', title: 'Настроить рабочее окружение', status: 'next' },
  ],
  // стартовое состояние стажёра: ничего ещё не пройдено
  onboarding: { done: 0, total: 7, nextStep: 'Познакомиться с командой' },
  contacts: [
    { id: 'mentor', name: 'Наставник', kind: 'mentor' },
    { id: 'hr', name: 'HR BP', kind: 'hr' },
    { id: 'team', name: 'Команда', kind: 'team' },
  ],
}

/** Ответ-заглушка вместо LLM в режиме mock. */
export const BUDDY_REPLY =
  'Это заглушка: бэкенд не подключён. Запусти фронт в режиме api, и здесь будет настоящий ответ по базе знаний.'
