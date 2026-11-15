// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from './api/types'
import App from './App'

const api = vi.hoisted(() => ({
  hasSession: vi.fn(),
  fetchMe: vi.fn(),
  logout: vi.fn(),
  onUnauthorized: vi.fn(),
}))
vi.mock('./api/client', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api/client')>()), ...api }))

// Экраны проверяются в своих тестах; здесь — только как App их переключает и что им передаёт.
vi.mock('./Login', () => ({
  Login: ({ onEnter }: { onEnter: (u: User) => void }) => (
    <button onClick={() => onEnter({ name: 'Анна', short: 'Анна С.', role: 'Наставник', kind: 'mentor' })}>войти</button>
  ),
}))
vi.mock('./Home', () => ({
  Home: ({ active, openChat }: { active: boolean; openChat: (d?: string) => void }) => (
    <div data-testid="home" data-active={String(active)}>
      <button onClick={() => openChat('Как выполнить задачу «Настроить VPN»?')}>спросить про задачу</button>
    </div>
  ),
}))
vi.mock('./Chat', () => ({
  Chat: (p: { active: boolean; question?: string; prefill?: string; onQuestionSent?: () => void; onPrefilled?: () => void }) => (
    <div data-testid="chat" data-active={String(p.active)}>
      <span>вопрос: {p.question ?? '—'}</span>
      <span>заготовка: {p.prefill ?? '—'}</span>
      <button onClick={p.onQuestionSent}>вопрос отправлен</button>
      <button onClick={p.onPrefilled}>заготовка принята</button>
    </div>
  ),
}))

const SASHA_API = { name: 'Саша', short: 'Саша И.', role: 'Стажёр', kind: 'intern' as const }

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
  window.history.replaceState(null, '', '/')
})
afterEach(cleanup)

const pane = (id: 'home' | 'chat') => screen.getByTestId(id).parentElement as HTMLElement

describe('App', () => {
  it('без сессии — экран входа; после входа — главная', async () => {
    api.hasSession.mockReturnValue(false)
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'войти' }))
    expect(pane('home').hidden).toBe(false)
    expect(pane('chat').hidden).toBe(true)
    expect(screen.getByText('Анна С.')).toBeTruthy()
    expect(api.fetchMe).not.toHaveBeenCalled()
  })

  it('сохранённая сессия восстанавливается через /api/me без мигания экрана входа', async () => {
    api.hasSession.mockReturnValue(true)
    api.fetchMe.mockResolvedValue(SASHA_API)
    const { container } = render(<App />)
    expect(container.innerHTML).toBe('')
    expect(await screen.findByText('Саша И.')).toBeTruthy()
  })

  it('протухшая сессия: выход и экран входа', async () => {
    api.hasSession.mockReturnValue(true)
    api.fetchMe.mockImplementation(() => Promise.reject(new Error('401')))
    render(<App />)
    expect(await screen.findByRole('button', { name: 'войти' })).toBeTruthy()
    expect(api.logout).toHaveBeenCalled()
  })

  it('401 на любом запросе возвращает экран входа; подписка снимается при размонтировании', async () => {
    api.hasSession.mockReturnValue(true)
    api.fetchMe.mockResolvedValue(SASHA_API)
    const view = render(<App />)
    await screen.findByText('Саша И.')
    const onUnauthorized = api.onUnauthorized.mock.calls[0][0] as () => void
    act(() => onUnauthorized())
    expect(screen.getByRole('button', { name: 'войти' })).toBeTruthy()
    view.unmount()
    expect(api.onUnauthorized).toHaveBeenLastCalledWith(null)
  })

  it('?s=chat открывает чат сразу после входа', async () => {
    window.history.replaceState(null, '', '/?s=chat')
    api.hasSession.mockReturnValue(true)
    api.fetchMe.mockResolvedValue(SASHA_API)
    render(<App />)
    await screen.findByText('Саша И.')
    expect(pane('chat').hidden).toBe(false)
    expect(screen.getByTestId('chat').dataset.active).toBe('true')
    expect(screen.getByTestId('home').dataset.active).toBe('false')
  })

  it('клик по задаче на главной открывает чат с заготовкой', async () => {
    api.hasSession.mockReturnValue(false)
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'войти' }))
    await userEvent.click(screen.getByRole('button', { name: 'спросить про задачу' }))
    expect(pane('chat').hidden).toBe(false)
    expect(screen.getByText('заготовка: Как выполнить задачу «Настроить VPN»?')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'заготовка принята' }))
    expect(screen.getByText('заготовка: —')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Главная' }))
    expect(pane('home').hidden).toBe(false)
  })

  it('«Что умеет Buddy»: пример из окна уходит в чат как вопрос', async () => {
    api.hasSession.mockReturnValue(false)
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'войти' }))
    await userEvent.click(screen.getByRole('button', { name: /Анна С\./ }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Что умеет Buddy/ }))
    expect(screen.getByRole('dialog')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Кто в моей команде?' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(pane('chat').hidden).toBe(false)
    expect(screen.getByText('вопрос: Кто в моей команде?')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'вопрос отправлен' }))
    expect(screen.getByText('вопрос: —')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: /Анна С\./ }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Что умеет Buddy/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('«Выйти» сбрасывает сессию и возвращает экран входа', async () => {
    api.hasSession.mockReturnValue(false)
    render(<App />)
    await userEvent.click(screen.getByRole('button', { name: 'войти' }))
    await userEvent.click(screen.getByRole('button', { name: 'Чат' }))
    await userEvent.click(screen.getByRole('button', { name: /Анна С\./ }))
    await userEvent.click(screen.getByRole('menuitem', { name: /Выйти/ }))
    expect(api.logout).toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'войти' })).toBeTruthy()
    // следующий вход снова на главную
    await userEvent.click(screen.getByRole('button', { name: 'войти' }))
    await waitFor(() => expect(pane('home').hidden).toBe(false))
  })
})
