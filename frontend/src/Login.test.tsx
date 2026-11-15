// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api/client'
import { Login } from './Login'

const api = vi.hoisted(() => ({ login: vi.fn() }))
vi.mock('./api/client', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api/client')>()), ...api }))

const SASHA = { name: 'Саша', short: 'Саша И.', role: 'Стажёр', kind: 'intern' as const }

beforeEach(() => {
  api.login.mockReset()
})
afterEach(cleanup)

describe('Login', () => {
  it('«Войти через Avito ID» входит тестовой учёткой по умолчанию', async () => {
    api.login.mockResolvedValue(SASHA)
    const onEnter = vi.fn()
    render(<Login onEnter={onEnter} />)
    await userEvent.click(screen.getByRole('button', { name: /Войти через Avito ID/ }))
    expect(api.login).toHaveBeenCalledWith(undefined)
    expect(onEnter).toHaveBeenCalledWith(SASHA)
    expect(screen.getByText(/тестовый прототип/)).toBeTruthy()
  })

  it('«Войти по email» открывает поле логина и входит под введённой учёткой', async () => {
    api.login.mockResolvedValue({ ...SASHA, name: 'Пётр' })
    const onEnter = vi.fn()
    render(<Login onEnter={onEnter} />)
    await userEvent.click(screen.getByRole('button', { name: /Войти по email/ }))
    const go = screen.getByRole('button', { name: 'Войти' })
    expect((go as HTMLButtonElement).disabled).toBe(true)
    await userEvent.type(screen.getByLabelText('Логин'), 'petya.intern{Enter}')
    expect(api.login).toHaveBeenCalledWith('petya.intern')
    expect(onEnter).toHaveBeenCalled()
  })

  it('пустой логин не отправляется', async () => {
    render(<Login onEnter={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: /Войти по email/ }))
    await userEvent.type(screen.getByLabelText('Логин'), '   {Enter}')
    expect(api.login).not.toHaveBeenCalled()
  })

  it('неизвестная учётка: подсказка с примером логина, кнопки снова активны', async () => {
    api.login.mockImplementation(() => Promise.reject(new ApiError(401, 'invalid login')))
    render(<Login onEnter={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: /Войти через Avito ID/ }))
    expect((await screen.findByRole('alert')).textContent).toBe('Такой учётки нет. Пример: petya.intern')
    expect((screen.getByRole('button', { name: /Войти через Avito ID/ }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('сервер недоступен: понятный текст', async () => {
    api.login.mockImplementation(() => Promise.reject(new ApiError(0, 'network')))
    render(<Login onEnter={vi.fn()} />)
    await userEvent.click(screen.getByRole('button', { name: /Войти через Avito ID/ }))
    expect((await screen.findByRole('alert')).textContent).toBe('Сервер недоступен. Проверь, что бэкенд запущен.')
  })

  it('масштаб экрана пересчитывается при изменении окна', () => {
    const { container } = render(<Login onEnter={vi.fn()} />)
    const wrap = container.querySelector('.login-wrap') as HTMLElement
    act(() => {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
      fireEvent(window, new Event('resize'))
    })
    expect(wrap.style.getPropertyValue('--k')).toBe('1')
    act(() => {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1536 })
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: 1152 })
      fireEvent(window, new Event('resize'))
    })
    expect(wrap.style.getPropertyValue('--k')).toBe('1.75')
  })
})
