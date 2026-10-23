import { useRef } from 'react'
import { useParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useResource } from '@/lib/hooks'
import { DateText, Loading, TagLinks } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/layout'
import {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetClose,
} from '@/components/ui/sheet'
import { Badge } from '@/components/ui/badge'
import { SnapshotView } from './markdown'
import { useArticleOutline } from './use-article-outline'

export function PageScreen({ home = false }: { home?: boolean }) {
  const { id: routeID = '' } = useParams(),
    id = home ? '1' : routeID
  const resource = useResource<Schema<'PageDetail'>>(`pages/${id}`)
  const tocTarget = useRef<string | null>(null)
  const article = useRef<HTMLDivElement>(null)
  const { headings, active } = useArticleOutline(article, resource.data)
  if (!resource.data) return <Loading error={resource.error} />
  const { page, revision, capabilities } = resource.data
  return (
    <>
      <PageHeader id={id} title={revision?.snapshot.title ?? '页面'} capabilities={capabilities} />
      {headings.length > 0 && (
        <div className="mobile-toc">
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="sm">
                文章目录
              </Button>
            </SheetTrigger>
            <SheetContent
              aria-describedby={undefined}
              onCloseAutoFocus={(event) => {
                const target = tocTarget.current && document.getElementById(tocTarget.current)
                tocTarget.current = null
                if (!target) return
                event.preventDefault()
                target.tabIndex = -1
                target.scrollIntoView({ block: 'start' })
                target.focus({ preventScroll: true })
              }}
            >
              <SheetHeader className="shrink-0">
                <SheetTitle>文章目录</SheetTitle>
              </SheetHeader>
              <nav className="toc-links" aria-label="文章目录">
                {headings.map((h) => (
                  <SheetClose asChild key={h.id}>
                    <a
                      href={'#' + h.id}
                      data-level={h.level}
                      aria-current={active === h.id ? 'location' : undefined}
                      onClick={() => {
                        tocTarget.current = h.id
                      }}
                    >
                      {h.text}
                    </a>
                  </SheetClose>
                ))}
              </nav>
            </SheetContent>
          </Sheet>
        </div>
      )}
      <div className="reading-grid">
        <article>
          <div className="reading-meta">
            <TagLinks tags={revision?.snapshot.tags ?? []} />
            {page.visibility === 'restricted' && <Badge variant="outline">受限</Badge>}
          </div>
          <div className="reading" ref={article}>
            {revision && (
              <SnapshotView snapshot={revision.snapshot} contentURL={revision.content_url} />
            )}
          </div>
          {revision && (
            <p className="metadata">
              更新于 <DateText value={revision.created_at} />
            </p>
          )}
        </article>
        {headings.length > 0 && (
          <aside className="article-toc">
            <h2>目录</h2>
            <nav aria-label="文章目录">
              {headings.map((h) => (
                <a
                  key={h.id}
                  href={'#' + h.id}
                  data-level={h.level}
                  aria-current={active === h.id ? 'location' : undefined}
                  onClick={() => document.getElementById(h.id)?.focus({ preventScroll: true })}
                >
                  {h.text}
                </a>
              ))}
            </nav>
          </aside>
        )}
      </div>
    </>
  )
}
