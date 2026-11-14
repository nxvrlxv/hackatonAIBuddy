// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api/client'
import type { ChatMessage, StreamHandlers, User } from './api/types'
import { Chat } from './Chat'

const api = vi.hoisted(() => ({
  fetchHistory: vi.fn(),
  sendMessage: vi.fn(),
  startNewChat: vi.fn(),
  cachedHistory: vi.fn(),
  rememberHistory: vi.fn(),
}))
vi.mock('./api/client', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api/client')>()), ...api }))

const SASHA: User = { name: 'Саша', short: 'Саша И.', role: 'Стажёр', kind: 'intern' }
const HISTORY: ChatMessage[] = [
  { id: 's1', role: 'user', text: 'Какие ценности у Авито?', at: '10:00' },
  { id: 's2', role: 'bot', text: 'Ценности описаны в **common/mission-and-values.md**.', at: '10:01' },
]

/** Ответ Buddy, которым тест управляет вручную: статус, кусок текста, завершение или ошибка. */
function controlledReply() {
  const ctl = {
    handlers: {} as StreamHandlers,
    resolve: (() => {}) as (m: ChatMessage) => void,
    reject: (() => {}) as (e: unknown) => void,
  }
  api.sendMessage.mockImplementation(
    (_text: string, h: StreamHandlers) =>
      new Promise<ChatMessage>((res, rej) => {
        ctl.handlers = h
        ctl.resolve = res
        ctl.reject = rej
      }),
  )
  return ctl
}

const reply = (text: string): ChatMessage => ({ id: 'b1', role: 'bot', text, at: '10:05' })

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
  api.cachedHistory.mockReturnValue(null)
  api.fetchHistory.mockResolvedValue([])
})
afterEach(cleanup)

function renderChat(props: Partial<Parameters<typeof Chat>[0]> = {}) {
  return render(<Chat user={SASHA} active {...props} />)
}

describe('Chat', () => {
  it('без истории — пустое состояние с подсказкой', async () => {
    renderChat()
    expect(await screen.findByText('Чем помочь?')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Начать заново' })).toBeNull()
    await waitFor(() => expect(api.rememberHistory).toHaveBeenCalledWith([]))
  })

  it('история из кэша видна сразу, а загруженная с сервера та же лента не перерисовывается', async () => {
    api.cachedHistory.mockReturnValue(HISTORY)
    api.fetchHistory.mockResolvedValue(HISTORY.map((m) => ({ ...m, id: `srv-${m.id}` })))
    const { container } = renderChat()
    expect(screen.getByText('Какие ценности у Авито?')).toBeTruthy()
    expect(container.querySelector('strong')?.textContent).toBe('common/mission-and-values.md')
    await waitFor(() => expect(api.fetchHistory).toHaveBeenCalled())
    expect(screen.getAllByText('Какие ценности у Авито?')).toHaveLength(1)
  })

  it('история с сервера заменяет пустой кэш', async () => {
    api.fetchHistory.mockResolvedValue(HISTORY)
    renderChat()
    expect(await screen.findByText('Какие ценности у Авито?')).toBeTruthy()
    expect(screen.getByText(/Саша/)).toBeTruthy()
  })

  it('история не загрузилась — понятная ошибка в пустом состоянии', async () => {
    api.fetchHistory.mockImplementation(() => Promise.reject(new ApiError(0, 'network')))
    renderChat()
    expect(await screen.findByText('Сервер недоступен. Проверь, что бэкенд запущен.')).toBeTruthy()
  })

  it('вопрос уходит по Enter, статус и текст ответа видны по мере прихода', async () => {
    const ctl = controlledReply()
    renderChat()
    await screen.findByText('Чем помочь?')
    const box = screen.getByLabelText('Сообщение')
    const send = screen.getByRole('button', { name: 'Отправить' }) as HTMLButtonElement
    expect(send.disabled).toBe(true)

    await userEvent.type(box, 'Как получить доступ к GitLab?{Enter}')
    expect(api.sendMessage).toHaveBeenCalledWith('Как получить доступ к GitLab?', expect.anything())
    expect(screen.getByText('Как получить доступ к GitLab?')).toBeTruthy()
    expect((box as HTMLTextAreaElement).value).toBe('')
    expect(screen.getAllByText(/печатает…/).length).toBeGreaterThan(0)

    act(() => ctl.handlers.onStatus?.('Ищу в базе знаний'))
    expect(screen.getByText('Ищу в базе знаний')).toBeTruthy()
    act(() => ctl.handlers.onText?.('Оформи **заявку**'))
    expect(screen.getByText('заявку').tagName).toBe('STRONG')

    // пока Buddy отвечает, второй вопрос не уходит
    await userEvent.type(box, 'ещё вопрос{Enter}')
    expect(api.sendMessage).toHaveBeenCalledTimes(1)

    await act(async () => ctl.resolve(reply('Оформи заявку в GitLab.')))
    expect(screen.getByText('Оформи заявку в GitLab.')).toBeTruthy()
    expect(screen.getByText('онлайн')).toBeTruthy()
  })

  it('Shift+Enter переносит строку, кнопка «Отправить» отправляет', async () => {
    const ctl = controlledReply()
    renderChat()
    await screen.findByText('Чем помочь?')
    const box = screen.getByLabelText('Сообщение') as HTMLTextAreaElement
    await userEvent.type(box, 'первая{Shift>}{Enter}{/Shift}вторая')
    expect(api.sendMessage).not.toHaveBeenCalled()
    expect(box.value).toBe('первая\nвторая')
    await userEvent.click(screen.getByRole('button', { name: 'Отправить' }))
    expect(api.sendMessage).toHaveBeenCalledWith('первая\nвторая', expect.anything())
    await act(async () => ctl.resolve(reply('ок')))
  })

  it('ошибка ответа: понятный текст и «Повторить» задаёт тот же вопрос', async () => {
    const ctl = controlledReply()
    renderChat()
    await screen.findByText('Чем помочь?')
    await userEvent.type(screen.getByLabelText('Сообщение'), 'Оформи отгул{Enter}')
    await act(async () => ctl.reject(new ApiError(0, 'timeout')))
    expect(screen.getByRole('alert').textContent).toContain('Buddy долго не отвечает. Попробуй ещё раз.')

    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(api.sendMessage).toHaveBeenCalledTimes(2)
    expect(api.sendMessage).toHaveBeenLastCalledWith('Оформи отгул', expect.anything())
    expect(screen.queryByRole('alert')).toBeNull()
    await act(async () => ctl.resolve(reply('Заявка подана.')))
    expect(screen.getByText('Заявка подана.')).toBeTruthy()
  })

  it('отмена запроса (AbortError) не показывает ошибку', async () => {
    const ctl = controlledReply()
    renderChat()
    await screen.findByText('Чем помочь?')
    await userEvent.type(screen.getByLabelText('Сообщение'), 'вопрос{Enter}')
    await act(async () => ctl.reject(new DOMException('aborted', 'AbortError')))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('«Начать заново» прерывает ответ, очищает ленту и открывает новый чат', async () => {
    api.fetchHistory.mockResolvedValue(HISTORY)
    const ctl = controlledReply()
    renderChat()
    await screen.findByText('Какие ценности у Авито?')
    await userEvent.type(screen.getByLabelText('Сообщение'), 'ещё{Enter}')
    const signal = ctl.handlers.signal as AbortSignal
    await userEvent.click(screen.getByRole('button', { name: 'Начать заново' }))
    expect(signal.aborted).toBe(true)
    expect(api.startNewChat).toHaveBeenCalled()
    expect(screen.getByText('Чем помочь?')).toBeTruthy()
    expect(screen.queryByText('Какие ценности у Авито?')).toBeNull()
  })

  it('вопрос из «Что умеет Buddy» отправляется сам после загрузки истории', async () => {
    const ctl = controlledReply()
    const onQuestionSent = vi.fn()
    renderChat({ question: 'Кто в моей команде?', onQuestionSent })
    await waitFor(() => expect(api.sendMessage).toHaveBeenCalledWith('Кто в моей команде?', expect.anything()))
    expect(onQuestionSent).toHaveBeenCalled()
    await act(async () => ctl.resolve(reply('Анна Смирнова — твой наставник.')))
  })

  it('заготовка с главной встаёт в поле ввода и не отправляется сама', async () => {
    const onPrefilled = vi.fn()
    renderChat({ prefill: 'Как выполнить задачу «Настроить VPN»?', onPrefilled })
    const box = (await screen.findByLabelText('Сообщение')) as HTMLTextAreaElement
    expect(box.value).toBe('Как выполнить задачу «Настроить VPN»?')
    expect(onPrefilled).toHaveBeenCalled()
    expect(api.sendMessage).not.toHaveBeenCalled()
    await waitFor(() => expect(document.activeElement).toBe(box))
  })

  it('при уходе с вкладки ответ продолжает приходить, при размонтировании запрос отменяется', async () => {
    const ctl = controlledReply()
    const view = renderChat()
    await screen.findByText('Чем помочь?')
    await userEvent.type(screen.getByLabelText('Сообщение'), 'вопрос{Enter}')
    view.rerender(<Chat user={SASHA} active={false} />)
    act(() => ctl.handlers.onText?.('часть ответа'))
    view.rerender(<Chat user={SASHA} active />)
    expect(screen.getByText('часть ответа')).toBeTruthy()
    const signal = ctl.handlers.signal as AbortSignal
    view.unmount()
    expect(signal.aborted).toBe(true)
  })

  it('пустой ввод из пробелов не отправляется', async () => {
    renderChat()
    await screen.findByText('Чем помочь?')
    fireEvent.submit(screen.getByLabelText('Сообщение').closest('form') as HTMLFormElement)
    expect(api.sendMessage).not.toHaveBeenCalled()
  })
})
