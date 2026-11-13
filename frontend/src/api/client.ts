import { BUDDY_REPLY, HOME } from './mock'
import type { ChatMessage, HomeData, User } from './types'

/**
 * Единственная точка общения с бэкендом.
 * Сейчас все методы возвращают локальные заглушки — сеть не используется.
 *
 * Когда API будет готов, меняется только тело функций:
 *   const r = await fetch('/api/chat', { method: 'POST', body: JSON.stringify({ message }) })
 *   if (!r.ok) throw new ApiError(...)
 *   return r.json()
 * Сигнатуры и типы при этом остаются прежними, экраны править не нужно.
 */

const LATENCY = 450

const delay = (ms = LATENCY) => new Promise<void>((res) => setTimeout(res, ms))

/** TODO(backend): заменить на GET /api/me */
export async function fetchMe(): Promise<User> {
  await delay(120)
  return HOME.user
}

/** TODO(backend): заменить на GET /api/home */
export async function fetchHome(): Promise<HomeData> {
  await delay(160)
  return HOME
}

/** TODO(backend): заменить на GET /api/chat/messages */
export async function fetchHistory(): Promise<ChatMessage[]> {
  await delay(140)
  return []
}

let seq = 0
export function newId(): string {
  seq += 1
  return `m${seq}`
}

export function now(): string {
  return new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

/** TODO(backend): заменить на POST /api/chat (SSE-стрим ответа) */
export async function sendMessage(_text: string): Promise<ChatMessage> {
  await delay(700)
  return { id: newId(), role: 'bot', text: BUDDY_REPLY, at: now() }
}

/** TODO(backend): заменить на POST /api/auth/login */
export async function login(): Promise<void> {
  await delay(300)
}
