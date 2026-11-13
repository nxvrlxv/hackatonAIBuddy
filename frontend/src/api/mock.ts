import type { HomeData } from './types'

/**
 * Временные данные вместо ответов API.
 * Когда бэкенд будет готов — эти константы удаляются, экраны берут данные из client.ts.
 */
export const HOME: HomeData = {
  user: { name: 'Александр', role: 'Стажёр' },
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

/** Ответ-заглушка вместо LLM. TODO(backend): убрать после подключения /api/chat. */
export const BUDDY_REPLY =
  'Это заглушка: бэкенд ещё не подключён. Как только появится API, здесь будет настоящий ответ по базе знаний.'
