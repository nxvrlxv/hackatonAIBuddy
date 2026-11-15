import type { FocusTask, HomeData, InternProgress, ReviewRequest, User, UserKind } from './types'

/**
 * Временные данные вместо ответов API.
 * Используются только в режиме mock (VITE_API_MODE=mock); в режиме api всё берётся из бэкенда.
 * Роль задаётся адресом: ?as=mentor — наставник с запросами на проверку, ?as=lead — руководитель.
 */
const USERS: Record<UserKind, User> = {
  intern: { name: 'Александр', short: 'Александр С.', role: 'Стажёр', kind: 'intern' },
  mentor: { name: 'Анна', short: 'Анна С.', role: 'Наставник', kind: 'mentor' },
  lead: { name: 'Игорь', short: 'Игорь Л.', role: 'Руководитель', kind: 'lead' },
  other: { name: 'Гость', short: 'Гость', role: 'Сотрудник', kind: 'other' },
}

function mockKind(): UserKind {
  const as = new URLSearchParams(globalThis.location?.search ?? '').get('as')
  return as === 'mentor' || as === 'lead' ? as : 'intern'
}

export function mockUser(): User {
  return USERS[mockKind()]
}
const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

// стажёр: одна задача на проверке, одна возвращена с комментарием, одна не начата
let focus: FocusTask[] = [
  { id: 'team', title: 'Познакомиться с командой', status: 'review', submittedAt: minutesAgo(25) },
  {
    id: 'gitlab',
    title: 'Получить доступ к GitLab',
    status: 'work',
    reviewComment: 'Нет доступа к репозиторию команды — запроси роль developer в группе travel.',
  },
  { id: 'env', title: 'Настроить рабочее окружение', status: 'next' },
]

const TEAM: HomeData['team'] = [
  { id: '1', name: 'Анна Смирнова', role: 'mentor', isMyMentor: true },
  { id: '2', name: 'Игорь Лидов', role: 'lead', isMyMentor: false },
  { id: '3', name: 'Дима Петров', role: 'intern', isMyMentor: false },
]

export function mockHome(): Omit<HomeData, 'user'> {
  const teamName = 'Команда Авито Путешествий'
  if (mockKind() !== 'intern')
    return { focus: [], onboarding: { done: 0, total: 0, nextStep: 'Адаптация пройдена' }, team: TEAM.slice(1), teamName }
  const next = focus.find((t) => t.status === 'work' || t.status === 'next')
  return {
    focus: focus.map((t) => ({ ...t })),
    // review в прогресс не идёт: принять задачу может только наставник
    onboarding: { done: 0, total: 7, nextStep: next?.title ?? 'Ждём проверки наставника' },
    team: TEAM,
    teamName,
  }
}

export function mockSubmit(id: string) {
  focus = focus.map((t) => (t.id === id ? { id: t.id, title: t.title, status: 'review', submittedAt: new Date().toISOString() } : t))
}

let reviews: ReviewRequest[] = [
  { taskId: '42', taskTitle: 'Доступ к GitLab', taskDescription: 'Оформи заявку на доступ к GitLab и репозиториям команды.', stageTitle: 'Доступы и инструменты', internId: '3', internName: 'Саша Иванов', submittedAt: minutesAgo(130) },
  { taskId: '43', taskTitle: 'Настроить рабочее окружение', taskDescription: 'Установи Go, Docker и склонируй сервис команды.', stageTitle: 'Доступы и инструменты', internId: '3', internName: 'Саша Иванов', submittedAt: minutesAgo(47) },
  { taskId: '51', taskTitle: 'Познакомиться с командой', taskDescription: 'Представься на дейли и запиши, кто за что отвечает.', stageTitle: 'Команда', internId: '4', internName: 'Дима Петров', submittedAt: minutesAgo(12) },
  { taskId: '57', taskTitle: 'Прочитать гайд по код-ревью', taskDescription: 'Изучи правила ревью и оформления merge request.', stageTitle: 'Процессы', internId: '4', internName: 'Дима Петров', submittedAt: minutesAgo(3) },
]

export function mockReviews(): ReviewRequest[] {
  return mockKind() === 'mentor' ? reviews.map((r) => ({ ...r })) : []
}

const INTERNS: Omit<InternProgress, 'inReview'>[] = [
  { id: '4', name: 'Дима Петров', done: 1, total: 11, percent: 9, stage: 'Знакомство с командой' },
  { id: '3', name: 'Саша Иванов', done: 5, total: 11, percent: 45, stage: 'Доступы и инструменты' },
  { id: '8', name: 'Олег Ветров', done: 11, total: 11, percent: 100, stage: '' },
]

/** Стажёры наставника; «на проверке» считается по текущим запросам — после принятия/отклонения число падает. */
export function mockInterns(): InternProgress[] {
  if (mockKind() !== 'mentor') return []
  return INTERNS.map((i) => ({ ...i, inReview: reviews.filter((r) => r.internId === i.id).length }))
}

/** Принять/отклонить: запрос уходит из списка. Нет такого — false (как 404). */
export function mockResolve(taskId: string): boolean {
  // не наставнику чужие задачи не видны — как 404 на бэкенде
  const had = mockKind() === 'mentor' && reviews.some((r) => r.taskId === taskId)
  if (had) reviews = reviews.filter((r) => r.taskId !== taskId)
  return had
}

/** Ответ-заглушка вместо LLM в режиме mock. */
export const BUDDY_REPLY =
  'Это заглушка: бэкенд не подключён. Запусти фронт в режиме api, и здесь будет настоящий ответ по базе знаний.'
