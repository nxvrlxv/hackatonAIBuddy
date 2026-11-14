/**
 * Разбор потока Server-Sent Events из fetch-ответа.
 * Формат агента: кадры через пустую строку, в кадре строки `event: <имя>` и `data: <json>`.
 */
export interface SseEvent {
  event: string
  data: unknown
}

/** Разбирает один кадр. Пустой кадр или кадр без data возвращает null. */
export function parseFrame(frame: string): SseEvent | null {
  let event = 'message'
  const data: string[] = []
  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  if (data.length === 0) return null
  const raw = data.join('\n')
  try {
    return { event, data: JSON.parse(raw) }
  } catch {
    return { event, data: raw }
  }
}

/** Читает поток до конца и отдаёт каждое событие в onEvent. */
export async function readSse(body: ReadableStream<Uint8Array>, onEvent: (e: SseEvent) => void): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
    let m: RegExpExecArray | null
    while ((m = /\r?\n\r?\n/.exec(buffer)) !== null) {
      const parsed = parseFrame(buffer.slice(0, m.index))
      buffer = buffer.slice(m.index + m[0].length)
      if (parsed) onEvent(parsed)
    }
    if (done) break
  }
  const tail = parseFrame(buffer)
  if (tail) onEvent(tail)
}
