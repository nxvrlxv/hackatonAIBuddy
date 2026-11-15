// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api/client'
import type { HomeData, User } from './api/types'
import { Home } from './Home'

const api = vi.hoisted(() => ({
  fetchHome: vi.fn(),
  submitForReview: vi.fn(),
  fetchReviews: vi.fn(),
  fetchInterns: vi.fn(),
  approveTask: vi.fn(),
}))
vi.mock('./api/client', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api/client')>()), ...api }))

const fail = (e: unknown) => () => Promise.reject(e)

const SASHA: User = { name: 'Саша', short: 'Саша И.', role: 'Стажёр', kind: 'intern' }
const ANNA: User = { name: 'Анна', short: 'Анна С.', role: 'Наставник', kind: 'mentor' }
const IGOR: User = { name: 'Игорь', short: 'Игорь Л.', role: 'Руководитель', kind: 'lead' }

const SASHA_HOME: Omit<HomeData, 'user'> = {
  focus: [
    { id: '15', title: 'Доступ к GitLab', status: 'work' },
    { id: '17', title: 'Настроить VPN', status: 'work', reviewComment: 'Проверь подключение из дома' },
    { id: '16', title: 'Доступ к Jira', status: 'review', submittedAt: new Date(Date.now() - 2 * 3600_000).toISOString() },
  ],
  onboarding: { done: 4, total: 11, nextStep: 'Доступ к GitLab' },
  team: [
    { id: '2', name: 'Анна Смирнова', role: 'mentor', isMyMentor: true },
    { id: '7', name: 'Игорь Лебедев', role: 'lead', isMyMentor: false },
    { id: '4', name: 'Дмитрий Орлов', role: 'intern', isMyMentor: false },
    { id: '9', name: 'Гость', role: 'designer', isMyMentor: false },
  ],
  teamName: 'Команда Авито Путешествий',
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
})
afterEach(cleanup)

function renderHome(user: User = SASHA, active = true) {
  const openChat = vi.fn()
  const view = render(<Home user={user} active={active} openChat={openChat} />)
  return { openChat, ...view }
}

describe('Home: стажёр', () => {
  it('текущие задачи с плашками, онбординг и команда', async () => {
    api.fetchHome.mockResolvedValue(SASHA_HOME)
    renderHome()
    expect(await screen.findByText('Привет, Саша!')).toBeTruthy()
    const focus = screen.getByText('Текущие задачи').closest('section') as HTMLElement
    expect(within(focus).getAllByRole('listitem')).toHaveLength(3)
    expect(screen.getByText('В работе')).toBeTruthy()
    expect(screen.getByText('Нужно доработать')).toBeTruthy()
    expect(screen.getByText('Наставник: Проверь подключение из дома')).toBeTruthy()
    const review = screen.getByText('На проверке').closest('.pill') as HTMLElement
    expect(review.getAttribute('title')).toBe('Отправлено наставнику 2 ч назад')
    expect(screen.getByText('4/11')).toBeTruthy()
    expect(screen.getByText('36%')).toBeTruthy()
    expect(screen.getByText('Следующий шаг')).toBeTruthy()
    expect(screen.getByText('Команда Авито Путешествий')).toBeTruthy()
    expect(screen.getByText('Твой наставник')).toBeTruthy()
    expect(screen.getByText('Руководитель')).toBeTruthy()
    expect(screen.getByText('Стажёр')).toBeTruthy()
    expect(screen.queryByText('Гость')).toBeNull()
    expect(screen.getByText(/и ещё 1 — спроси Buddy/)).toBeTruthy()
    // на проверке отправить ещё раз нельзя: кнопки только у задач «в работе»
    expect(screen.getAllByRole('button', { name: /Отправить на проверку/ })).toHaveLength(2)
  })

  it('клик по задаче, «Следующий шаг» и «Открыть чат» открывают чат с нужной заготовкой', async () => {
    api.fetchHome.mockResolvedValue(SASHA_HOME)
    const { openChat } = renderHome()
    await userEvent.click(await screen.findByText('Настроить VPN'))
    expect(openChat).toHaveBeenLastCalledWith('Как выполнить задачу «Настроить VPN»?')
    await userEvent.click(screen.getByRole('button', { name: /Следующий шаг/ }))
    expect(openChat).toHaveBeenLastCalledWith('Как выполнить задачу «Доступ к GitLab»?')
    await userEvent.click(screen.getByRole('button', { name: /Открыть чат/ }))
    expect(openChat).toHaveBeenLastCalledWith()
  })

  it('кнопка-галочка отправляет задачу на проверку: плашка меняется сразу, главная перечитывается', async () => {
    const after = { ...SASHA_HOME, focus: SASHA_HOME.focus.map((t) => (t.id === '15' ? { ...t, status: 'review' as const } : t)) }
    api.fetchHome.mockResolvedValueOnce(SASHA_HOME).mockResolvedValue(after)
    let finish = (): void => {}
    api.submitForReview.mockImplementation(() => new Promise((res) => (finish = () => res({ id: '15', title: 'Доступ к GitLab', status: 'review' }))))
    const { openChat } = renderHome()
    await userEvent.click(await screen.findByRole('button', { name: 'Отправить на проверку: Доступ к GitLab' }))
    expect(api.submitForReview).toHaveBeenCalledWith('15')
    expect(openChat).not.toHaveBeenCalled()
    // пока запрос идёт, остальные кнопки отправки неактивны
    expect((screen.getByRole('button', { name: 'Отправить на проверку: Настроить VPN' }) as HTMLButtonElement).disabled).toBe(true)
    finish()
    await waitFor(() => expect(screen.getAllByText('На проверке')).toHaveLength(2))
    await waitFor(() => expect(api.fetchHome).toHaveBeenCalledTimes(2))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('ошибка отправки показывается под задачами', async () => {
    api.fetchHome.mockResolvedValue(SASHA_HOME)
    api.submitForReview.mockImplementation(fail(new ApiError(409, 'task already approved')))
    renderHome()
    await userEvent.click(await screen.findByRole('button', { name: 'Отправить на проверку: Настроить VPN' }))
    expect((await screen.findByRole('alert')).textContent).toBe('Задачу уже приняли.')
  })

  it('адаптация пройдена: «Следующий шаг» открывает пустой чат', async () => {
    api.fetchHome.mockResolvedValue({
      ...SASHA_HOME,
      focus: [{ id: '20', title: 'Пройти первое ревью', status: 'done' }],
      onboarding: { done: 11, total: 11, nextStep: 'Адаптация пройдена' },
    })
    const { openChat } = renderHome()
    expect(await screen.findByText('Готово')).toBeTruthy()
    expect(screen.getByText('100%')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Отправить на проверку/ })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: /Следующий шаг/ }))
    expect(openChat).toHaveBeenLastCalledWith(undefined)
  })

  it('не загрузилось — сообщение; неактивная вкладка не грузит данные', async () => {
    api.fetchHome.mockImplementation(fail(new ApiError(0, 'network')))
    renderHome()
    expect(await screen.findByText('Не получилось загрузить данные. Обнови страницу.')).toBeTruthy()
    cleanup()
    api.fetchHome.mockClear()
    renderHome(SASHA, false)
    expect(api.fetchHome).not.toHaveBeenCalled()
  })
})

describe('Home: другие роли', () => {
  it('руководитель: пояснение вместо чек-листа и «Спросить Buddy»; пустая команда', async () => {
    api.fetchHome.mockResolvedValue({ focus: [], onboarding: { done: 0, total: 0, nextStep: '' }, team: [], teamName: 'Команда' })
    const { openChat } = renderHome(IGOR)
    expect(await screen.findByText(/Чек-лист адаптации ведут стажёры/)).toBeTruthy()
    expect(screen.getByText('Адаптация тебе не назначена — ты уже в команде.')).toBeTruthy()
    expect(screen.getByText('Пока в команде только ты.')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: /Спросить Buddy/ }))
    expect(openChat).toHaveBeenLastCalledWith(undefined)
  })

  it('наставник: запросы на проверку и мои стажёры; принятие перечитывает прогресс стажёров', async () => {
    api.fetchHome.mockResolvedValue({ focus: [], onboarding: { done: 0, total: 0, nextStep: '' }, team: [], teamName: 'Команда' })
    api.fetchReviews.mockResolvedValue([
      {
        taskId: '16',
        taskTitle: 'Доступ к Jira',
        taskDescription: '',
        stageTitle: '',
        internId: '3',
        internName: 'Саша Иванов',
        submittedAt: new Date().toISOString(),
      },
    ])
    api.fetchInterns.mockResolvedValue([])
    renderHome(ANNA)
    expect(await screen.findByText('Запросы на проверку')).toBeTruthy()
    expect(screen.getByText('Мои стажёры')).toBeTruthy()
    expect(screen.queryByText('Текущие задачи')).toBeNull()
    expect(screen.queryByText(/Чек-лист адаптации ведут стажёры/)).toBeNull()
    await screen.findByText('Доступ к Jira')
    await waitFor(() => expect(api.fetchInterns).toHaveBeenCalledTimes(1))
    api.approveTask.mockResolvedValue(undefined)
    await userEvent.click(screen.getByRole('button', { name: /Принять/ }))
    expect(api.approveTask).toHaveBeenCalledWith('16')
    await waitFor(() => expect(api.fetchInterns).toHaveBeenCalledTimes(2))
  })
})
