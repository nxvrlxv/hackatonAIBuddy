// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { Markdown } from './Markdown'
import { Avatar, AvitoMark, Icon, Icon3d, Logo, Mascot } from './ui'

afterEach(cleanup)

describe('Markdown', () => {
  it('жирный, списки, ссылка в новой вкладке и таблица в прокручиваемой обёртке', () => {
    const text = '**Важно**\n\n- раз\n- два\n\n[Плейбук](https://github.com/avito-tech/playbook)\n\n| a | b |\n|---|---|\n| 1 | 2 |'
    const { container } = render(<Markdown text={text} />)
    expect(container.querySelector('strong')?.textContent).toBe('Важно')
    expect(container.querySelectorAll('li')).toHaveLength(2)
    const link = screen.getByRole('link', { name: 'Плейбук' })
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toBe('noopener noreferrer')
    expect(container.querySelector('.md-table table')).not.toBeNull()
  })

  it('сырой HTML из ответа модели не исполняется', () => {
    const { container } = render(<Markdown text={'<img src=x onerror="alert(1)"><script>alert(1)</script>текст'} />)
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
  })
})

describe('ui', () => {
  it('иконка красится маской из /icons, картинки берутся из /img', () => {
    const { container } = render(
      <>
        <Icon name="check" size={14} />
        <AvitoMark size={57} />
        <Logo />
        <Mascot variant="wave" alt="котик" />
        <Avatar />
        <Icon3d tone="violet">x</Icon3d>
      </>,
    )
    const icon = container.querySelector('span[aria-hidden="true"]') as HTMLElement
    expect(icon.style.width).toBe('14px')
    expect(icon.getAttribute('style')).toContain('/icons/14-check-green.svg')
    expect(screen.getByAltText('Avito').getAttribute('src')).toBe('/img/avito-logo.png')
    expect(screen.getByAltText('котик').getAttribute('src')).toBe('/img/buddy-wave.webp')
    expect(container.querySelector('img[src="/icons/avito-mark.svg"]')?.getAttribute('height')).toBe('47')
    expect(container.querySelector('.avatar')).not.toBeNull()
    expect(container.querySelector('.ico-violet')?.textContent).toBe('x')
  })
})
