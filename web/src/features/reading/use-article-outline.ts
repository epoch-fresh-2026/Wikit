import { useEffect, useState, type RefObject } from 'react'

export function useArticleOutline(article: RefObject<HTMLDivElement | null>, content: unknown) {
  const [headings, setHeadings] = useState<{ id: string; text: string; level: number }[]>([])
  const [active, setActive] = useState('')

  useEffect(() => {
    const nodes = Array.from(article.current?.querySelectorAll<HTMLElement>('h1,h2,h3') ?? [])
    setHeadings(
      nodes.map((node, index) => {
        node.id = `section-${index}`
        node.tabIndex = -1
        return { id: node.id, text: node.textContent ?? '', level: Number(node.tagName[1]) }
      }),
    )
    let frame = 0
    const update = () => {
      frame = 0
      const above = nodes.filter((node) => node.getBoundingClientRect().top <= 180)
      setActive((above.at(-1) ?? nodes[0])?.id ?? '')
    }
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update)
    }
    update()
    // The article arrives asynchronously, after the browser's initial hash lookup.
    const target = nodes.find((node) => `#${node.id}` === window.location.hash)
    if (target) {
      target.scrollIntoView({ block: 'start' })
      target.focus({ preventScroll: true })
    }
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
    }
  }, [article, content])
  return { headings, active }
}
