// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Help } from './Help'
import { Shell } from './Shell'

const USER = { name: 'Анна', short: 'Анна С.', role: 'Наставник', kind: 'mentor' as const }

afterEach(cleanup)

function renderShell(over: Partial<Parameters<typeof Shell>[0]> = {}) {
  const props = { tab: 'main' as const, onTab: vi.fn(), user: USER, onHelp: vi.fn(), onLogout: vi.fn(), ...over }
  render(
    <Shell {...props}>
      <p>содержимое вкладки</p>
    </Shell>,
  )
  return props
}

describe('Shell', () => {
  it('вкладки: активная отмечена, клик переключает', async () => {
    const p = renderShell()
    expect(screen.getByRole('button', { name: 'Главная' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('button', { name: 'Чат' }).getAttribute('aria-current')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Чат' }))
    expect(p.onTab).toHaveBeenCalledWith('chat')
    expect(screen.getByText('содержимое вкладки')).toBeTruthy()
  })

  it('меню пользователя: «Что умеет Buddy» и «Выйти»', async () => {
    const p = renderShell()
    const userBtn = screen.getByRole('button', { name: /Анна С\./ })
    expect(screen.queryByRole('menu')).toBeNull()
    await userEvent.click(userBtn)
    expect(userBtn.getAttribute('aria-expanded')).toBe('true')
    await userEvent.click(screen.getByRole('menuitem', { name: /Что умеет Buddy/ }))
    expect(p.onHelp).toHaveBeenCalled()
    expect(screen.queryByRole('menu')).toBeNull()

    await userEvent.click(userBtn)
    await userEvent.click(screen.getByRole('menuitem', { name: /Выйти/ }))
    expect(p.onLogout).toHaveBeenCalled()
  })

  it('меню закрывается по Escape и по клику мимо', async () => {
    renderShell()
    const userBtn = screen.getByRole('button', { name: /Анна С\./ })
    await userEvent.click(userBtn)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menu')).toBeNull()

    await userEvent.click(userBtn)
    fireEvent.pointerDown(screen.getByRole('menu'))
    expect(screen.getByRole('menu')).toBeTruthy()
    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('menu')).toBeNull()

    await userEvent.click(userBtn)
    await userEvent.click(userBtn)
    expect(screen.queryByRole('menu')).toBeNull()
  })
})

describe('Help', () => {
  it('клик по примеру задаёт вопрос Buddy', async () => {
    const onAsk = vi.fn()
    render(<Help onAsk={onAsk} onClose={vi.fn()} />)
    expect(screen.getByRole('dialog', { name: 'Что умеет Buddy' })).toBeTruthy()
    for (const title of ['База знаний', 'Адаптация', 'Команда', 'Отгулы']) expect(screen.getByText(title)).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Оформи отгул на завтра' }))
    expect(onAsk).toHaveBeenCalledWith('Оформи отгул на завтра')
  })

  it('закрывается крестиком, Escape и кликом по фону, но не кликом по окну', async () => {
    const onClose = vi.fn()
    const { container } = render(<Help onAsk={vi.fn()} onClose={onClose} />)
    await userEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
    fireEvent.keyDown(document, { key: 'Enter' })
    expect(onClose).toHaveBeenCalledTimes(2)
    fireEvent.pointerDown(screen.getByRole('dialog'))
    expect(onClose).toHaveBeenCalledTimes(2)
    fireEvent.pointerDown(container.querySelector('.modal-backdrop') as Element)
    expect(onClose).toHaveBeenCalledTimes(3)
  })
})
