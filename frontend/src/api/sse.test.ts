import { describe, expect, it } from 'vitest'
import { parseFrame, readSse, type SseEvent } from './sse'

function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
  const enc = new TextEncoder()
  return new ReadableStream({
    start(c) {
      for (const ch of chunks) c.enqueue(enc.encode(ch))
      c.close()
    },
  })
}

describe('parseFrame', () => {
  it('разбирает имя события и JSON', () => {
    expect(parseFrame('event: text_delta\ndata: {"text":"При"}')).toEqual({ event: 'text_delta', data: { text: 'При' } })
  })

  it('склеивает несколько строк data', () => {
    expect(parseFrame('data: {"a":\ndata: 1}')).toEqual({ event: 'message', data: { a: 1 } })
  })

  it('кадр без data пропускается', () => {
    expect(parseFrame(': ping')).toBeNull()
    expect(parseFrame('')).toBeNull()
  })

  it('не-JSON отдаётся строкой', () => {
    expect(parseFrame('event: x\ndata: просто текст')).toEqual({ event: 'x', data: 'просто текст' })
  })
})

describe('readSse', () => {
  it('собирает кадры, разрезанные между кусками потока, и понимает CRLF', async () => {
    const got: SseEvent[] = []
    await readSse(
      streamOf(['event: status\r\ndata: {"te', 'xt":"ok"}\r\n\r\nevent: done\ndata: {}', '\n\n']),
      (e) => got.push(e),
    )
    expect(got).toEqual([
      { event: 'status', data: { text: 'ok' } },
      { event: 'done', data: {} },
    ])
  })

  it('последний кадр без пустой строки в конце тоже доходит', async () => {
    const got: SseEvent[] = []
    await readSse(streamOf(['event: done\ndata: {"run_id":"r1"}']), (e) => got.push(e))
    expect(got).toEqual([{ event: 'done', data: { run_id: 'r1' } }])
  })
})
