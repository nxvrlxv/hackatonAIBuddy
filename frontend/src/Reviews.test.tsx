// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api/client'
import type { InternProgress, ReviewRequest } from './api/types'
import { Interns } from './Interns'
import { Reviews } from './Reviews'

const api = vi.hoisted(() => ({
  fetchReviews: vi.fn(),
  approveTask: vi.fn(),
  rejectTask: vi.fn(),
  fetchInterns: vi.fn(),
}))
vi.mock('./api/client', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api/client')>()), ...api }))

const fail = (e: unknown) => () => Promise.reject(e)

const JIRA: ReviewRequest = {
  taskId: '16',
  taskTitle: 'Доступ к Jira',
  taskDescription: 'Получи доступ к доске команды в Jira.',
  stageTitle: 'Доступы и инструменты',
  internId: '3',
  internName: 'Саша Иванов',
  submittedAt: new Date(Date.now() - 20 * 60_000).toISOString(),
}
const GITLAB: ReviewRequest = { ...JIRA, taskId: '15', taskTitle: 'Доступ к GitLab', stageTitle: '', taskDescription: '' }

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
})
afterEach(cleanup)

describe('Reviews', () => {
  it('список запросов: стажёр, задача, этап, время и счётчик', async () => {
    api.fetchReviews.mockResolvedValue([JIRA, GITLAB])
    render(<Reviews active />)
    expect(screen.getByText('Загружаю…')).toBeTruthy()
    expect(await screen.findByText('Доступ к Jira')).toBeTruthy()
    expect(screen.getByText(/Доступы и инструменты/)).toBeTruthy()
    expect(screen.getAllByText('Саша Иванов')).toHaveLength(2)
    expect(screen.getAllByText('20 мин назад')).toHaveLength(2)
    expect(document.querySelector('.reviews-count')?.textContent).toBe('2')
  })

  it('пусто и ошибка загрузки — понятные тексты; неактивная вкладка не грузит', async () => {
    api.fetchReviews.mockResolvedValue([])
    const { unmount } = render(<Reviews active />)
    expect(await screen.findByText(/Новых запросов нет/)).toBeTruthy()
    unmount()

    api.fetchReviews.mockImplementation(fail(new ApiError(0, 'network')))
    const second = render(<Reviews active />)
    expect(await screen.findByText(/Не получилось загрузить запросы/)).toBeTruthy()
    second.unmount()

    api.fetchReviews.mockClear()
    render(<Reviews active={false} />)
    expect(api.fetchReviews).not.toHaveBeenCalled()
  })

  it('«Принять» убирает запрос, показывает итог и сообщает главной', async () => {
    api.fetchReviews.mockResolvedValue([JIRA, GITLAB])
    api.approveTask.mockResolvedValue(undefined)
    const onResolved = vi.fn()
    render(<Reviews active onResolved={onResolved} />)
    await screen.findByText('Доступ к GitLab')
    await userEvent.click(screen.getAllByRole('button', { name: /Принять/ })[1])
    expect(api.approveTask).toHaveBeenCalledWith('15')
    expect((await screen.findByRole('status')).textContent).toBe('Принято: «Доступ к GitLab» — Саша Иванов.')
    expect(screen.queryByText('Доступ к GitLab')).toBeNull()
    expect(onResolved).toHaveBeenCalledTimes(1)
  })

  it('«Отклонить»: без комментария отправить нельзя, с комментарием задача уходит на доработку', async () => {
    api.fetchReviews.mockResolvedValue([JIRA])
    api.rejectTask.mockResolvedValue(undefined)
    const onResolved = vi.fn()
    render(<Reviews active onResolved={onResolved} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Отклонить' }))
    const send = screen.getByRole('button', { name: 'Вернуть на доработку' }) as HTMLButtonElement
    expect(send.disabled).toBe(true)
    const box = screen.getByLabelText('Комментарий для стажёра')
    expect(document.activeElement).toBe(box)
    await userEvent.type(box, '   ')
    expect(send.disabled).toBe(true)
    await userEvent.type(box, 'Нет доступа к доске команды')
    expect(screen.getByText('30/500')).toBeTruthy()
    await userEvent.click(send)
    expect(api.rejectTask).toHaveBeenCalledWith('16', '   Нет доступа к доске команды')
    expect((await screen.findByRole('status')).textContent).toBe('Вернули на доработку: «Доступ к Jira» — Саша Иванов.')
    expect(screen.queryByLabelText('Комментарий для стажёра')).toBeNull()
    expect(onResolved).toHaveBeenCalled()
  })

  it('форма отклонения закрывается «Отмена», Escape и повторным «Отклонить»; Ctrl+Enter отправляет', async () => {
    api.fetchReviews.mockResolvedValue([JIRA])
    api.rejectTask.mockResolvedValue(undefined)
    render(<Reviews active />)
    const toggle = await screen.findByRole('button', { name: 'Отклонить' })

    await userEvent.click(toggle)
    await userEvent.click(screen.getByRole('button', { name: 'Отмена' }))
    expect(screen.queryByLabelText('Комментарий для стажёра')).toBeNull()

    await userEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    await userEvent.click(toggle)
    expect(screen.queryByLabelText('Комментарий для стажёра')).toBeNull()

    await userEvent.click(toggle)
    fireEvent.keyDown(screen.getByLabelText('Комментарий для стажёра'), { key: 'Escape' })
    expect(screen.queryByLabelText('Комментарий для стажёра')).toBeNull()

    await userEvent.click(toggle)
    const box = screen.getByLabelText('Комментарий для стажёра')
    await userEvent.type(box, 'Доделай')
    fireEvent.keyDown(box, { key: 'Enter' })
    expect(api.rejectTask).not.toHaveBeenCalled()
    fireEvent.keyDown(box, { key: 'Enter', ctrlKey: true })
    await waitFor(() => expect(api.rejectTask).toHaveBeenCalledWith('16', 'Доделай'))
  })

  it('ошибка сохранения остаётся под запросом, запрос не пропадает', async () => {
    api.fetchReviews.mockResolvedValue([JIRA])
    api.approveTask.mockImplementation(fail(new ApiError(500, 'internal error')))
    render(<Reviews active />)
    await userEvent.click(await screen.findByRole('button', { name: /Принять/ }))
    expect((await screen.findByRole('alert')).textContent).toBe('Не получилось сохранить. Попробуй ещё раз.')
    expect(screen.getByText('Доступ к Jira')).toBeTruthy()

    api.approveTask.mockResolvedValue(undefined)
    await userEvent.click(screen.getByRole('button', { name: /Принять/ }))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })

  it('запрос уже обработан (404 или 409): убирается из списка с пояснением', async () => {
    api.fetchReviews.mockResolvedValue([JIRA, GITLAB])
    api.approveTask.mockImplementation(fail(new ApiError(409, 'task is not in review')))
    api.rejectTask.mockImplementation(fail(new ApiError(404, 'not found')))
    const onResolved = vi.fn()
    render(<Reviews active onResolved={onResolved} />)
    await screen.findByText('Доступ к Jira')
    await userEvent.click(screen.getAllByRole('button', { name: /Принять/ })[0])
    expect((await screen.findByRole('status')).textContent).toBe('«Доступ к Jira»: Задачу уже проверили.')
    expect(screen.queryByText('Доступ к Jira')).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: 'Отклонить' }))
    await userEvent.type(screen.getByLabelText('Комментарий для стажёра'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Вернуть на доработку' }))
    expect((await screen.findByRole('status')).textContent).toBe(
      '«Доступ к GitLab»: Запрос не найден — возможно, его уже обработали.',
    )
    expect(onResolved).toHaveBeenCalledTimes(2)
    expect(screen.getByText(/Новых запросов нет/)).toBeTruthy()
  })
})

const intern = (over: Partial<InternProgress>): InternProgress => ({
  id: '3',
  name: 'Саша Иванов',
  done: 4,
  total: 11,
  percent: 36,
  inReview: 0,
  stage: 'Доступы и инструменты',
  ...over,
})

describe('Interns', () => {
  it('прогресс каждого стажёра, задачи на проверке, пройденная адаптация и «и ещё N»', async () => {
    api.fetchInterns.mockResolvedValue([
      intern({ inReview: 2 }),
      intern({ id: '4', name: 'Дмитрий Орлов', done: 11, total: 11, percent: 100, stage: '' }),
      intern({ id: '5', name: 'Новый Стажёр', done: 0, total: 11, percent: 0, stage: '' }),
      intern({ id: '6', name: 'Четвёртый' }),
    ])
    render(<Interns active refresh={0} />)
    expect(screen.getByText('Загружаю…')).toBeTruthy()
    expect(await screen.findByText('Саша Иванов')).toBeTruthy()
    expect(screen.getByText('4/11 · 36%')).toBeTruthy()
    expect(screen.getByText('2 на проверке')).toBeTruthy()
    expect(screen.getByText('Адаптация пройдена')).toBeTruthy()
    expect(screen.getByText('Ещё не начал')).toBeTruthy()
    expect(screen.queryByText('Четвёртый')).toBeNull()
    expect(screen.getByText('и ещё 1')).toBeTruthy()
    expect(screen.getAllByRole('progressbar')[0].getAttribute('aria-valuenow')).toBe('36')
  })

  it('нет стажёров, ошибка, перечитывание по refresh и неактивная вкладка', async () => {
    api.fetchInterns.mockResolvedValue([])
    const { rerender } = render(<Interns active refresh={0} />)
    expect(await screen.findByText(/За тобой пока не закреплены стажёры/)).toBeTruthy()

    api.fetchInterns.mockResolvedValue([intern({})])
    rerender(<Interns active refresh={1} />)
    expect(await screen.findByText('Саша Иванов')).toBeTruthy()
    expect(api.fetchInterns).toHaveBeenCalledTimes(2)

    rerender(<Interns active={false} refresh={2} />)
    expect(api.fetchInterns).toHaveBeenCalledTimes(2)
    cleanup()

    api.fetchInterns.mockImplementation(fail(new ApiError(403, 'forbidden')))
    render(<Interns active refresh={0} />)
    expect(await screen.findByText(/Не получилось загрузить стажёров/)).toBeTruthy()
  })
})
