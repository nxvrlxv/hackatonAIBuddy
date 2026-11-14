import { useEffect, useState } from 'react'

/**
 * Экран входа собран в рамке макета 768 × 576 (кадр 523 × 392 из макета, умноженный на 1,4685)
 * и целиком масштабируется под окно через zoom: текст и картинки перерисовываются в новом
 * размере, а не растягиваются. Масштаб ограничен 0,9…1,75, чтобы шрифт не мельчал, а котик
 * (исходник 900 px) оставался чётким. На узком экране (< 740) рамки нет, работает обычная вёрстка.
 */
export const FRAME_W = 768
export const FRAME_H = 576
export const MOBILE_MAX = 739

export function loginScale(w: number, h: number): number {
  if (w <= MOBILE_MAX) return 1
  const k = Math.min(w / FRAME_W, h / FRAME_H)
  // вниз, а не к ближайшему: рамка не должна выйти за окно на долю пикселя (иначе мелькнёт полоса прокрутки)
  return Math.floor(Math.min(1.75, Math.max(0.9, k)) * 1000) / 1000
}

const readScale = () => (typeof window === 'undefined' ? 1 : loginScale(window.innerWidth, window.innerHeight))

export function useLoginScale(): number {
  const [k, setK] = useState(readScale)
  useEffect(() => {
    const onResize = () => setK(readScale())
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return k
}
