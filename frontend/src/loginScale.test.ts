import { describe, expect, it } from 'vitest'
import { loginScale } from './loginScale'

describe('loginScale', () => {
  it('рамка макета 768 × 576 вписывается в окно по меньшей стороне', () => {
    expect(loginScale(768, 576)).toBe(1)
    expect(loginScale(1024, 768)).toBe(1.333)
    expect(loginScale(1440, 860)).toBe(1.493)
  })

  it('на большом мониторе масштаб не больше 1,75', () => {
    expect(loginScale(2560, 1440)).toBe(1.75)
  })

  it('в низком окне масштаб не меньше 0,9: шрифт не мельчает, страница прокручивается', () => {
    expect(loginScale(844, 390)).toBe(0.9)
  })

  it('на телефоне рамки нет, масштаб 1', () => {
    expect(loginScale(390, 844)).toBe(1)
    expect(loginScale(739, 1000)).toBe(1)
  })
})
