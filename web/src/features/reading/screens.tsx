import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { type Schema, query } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import {
  Collection,
  DateText,
  Loading,
  SectionTitle,
  SelectField,
  TagLinks,
  ActionFeedback,
} from '@/components/shared'
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
import { useAuth } from '@/features/account/auth'
import { SnapshotEditor, emptySnapshot } from '@/features/editing/editor'
import { SnapshotView } from './markdown'
import { SearchFilters } from './search-filters'

export function SearchScreen() {
  const auth = useAuth()
  const [params, setParams] = useSearchParams()
  const search = (params.get('q') ?? '').trim()
  const type = params.get('type') ?? ''
  const filtered =
    params.getAll('tags').length > 0 ||
    !!params.get('tag') ||
    params.getAll('exclude_tags').length > 0
  const request = new URLSearchParams(params)
  request.delete('q')
  request.delete('cursor')
  if (search) request.set('q', search)
  const path = (search ? 'search?' : 'pages?') + request.toString()
  return (
    <>
      <SectionTitle title={search ? '搜索结果' : '搜索页面'} />
      <SearchFilters key={params.toString()} params={params} apply={(value) => setParams(value)} />
      {search ? (
        <Collection<Schema<'SearchHit'>>
          key={path}
          path={path}
          empty={
            <>
              未找到匹配页面。
              {!filtered && !type && auth.account?.can_create_page && (
                <Button variant="link" asChild>
                  <Link to={`/pages/new?${query({ title: search })}`}>创建页面</Link>
                </Button>
              )}
            </>
          }
        >
          {(hit) => (
            <article className="directory-row" key={hit.page_id}>
              <h2>
                <Link to={'/pages/' + hit.page_id}>{hit.title}</Link>
              </h2>
              <p>{hit.snippet}</p>
              <TagLinks tags={hit.tags} />
            </article>
          )}
        </Collection>
      ) : (
        <Collection<Schema<'PageDetail'>> key={path} path={path}>
          {(item) => (
            <article className="directory-row" key={item.page.id}>
              <h2>
                <Link to={'/pages/' + item.page.id}>{item.revision?.snapshot.title}</Link>
              </h2>
              <p className="muted">
                {item.page.type === 'document' ? '文档' : '媒体'} · 页面 {item.page.id}
              </p>
              {item.revision && (
                <>
                  <TagLinks tags={item.revision.snapshot.tags} />
                  <small>
                    <DateText value={item.revision.created_at} />
                  </small>
                </>
              )}
            </article>
          )}
        </Collection>
      )}
    </>
  )
}
export function PageScreen({ home = false }: { home?: boolean }) {
  const { id: routeID = '' } = useParams(),
    id = home ? '1' : routeID
  const resource = useResource<Schema<'PageDetail'>>(`pages/${id}`)
  const tocTarget = useRef<string | null>(null)
  const article = useRef<HTMLDivElement>(null),
    [headings, setHeadings] = useState<{ id: string; text: string }[]>([])
  useEffect(() => {
    const nodes = article.current?.querySelectorAll('h1,h2,h3') ?? []
    setHeadings(
      Array.from(nodes).map((node, i) => {
        const id = 'section-' + i
        node.id = id
        return { id, text: node.textContent ?? '' }
      }),
    )
  }, [resource.data])
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
                <a key={h.id} href={'#' + h.id}>
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
export function EditPageScreen({ create = false }: { create?: boolean }) {
  const { id = '' } = useParams(),
    [params] = useSearchParams(),
    auth = useAuth(),
    navigate = useNavigate(),
    action = useAction()
  const resource = useResource<Schema<'PageDetail'>>(create ? null : `pages/${id}`)
  const [visibility, setVisibility] = useState('public')
  if (!auth.account)
    return (
      <p>
        请先<Link to="/login">登录</Link>，再提交提案。
      </p>
    )
  if (create && !auth.account.can_create_page) return <p role="alert">没有创建页面的权限。</p>
  if (!create && !resource.data) return <Loading error={resource.error} />
  if (!create && !resource.data?.revision)
    return (
      <p>
        页面尚未发布，请从<Link to={`/pages/${id}/proposals`}>创建提案</Link>继续编辑。
      </p>
    )
  return (
    <section>
      {create ? (
        <SectionTitle title="创建页面" />
      ) : (
        <PageHeader id={id} title="提出修改" capabilities={resource.data?.capabilities} />
      )}
      <p className="muted">
        {create ? '创建后立即发布首个版本。' : '提交提案，由有合并权限的人检查并发布。'}
      </p>
      <div className="panel stack">
        {create && (
          <SelectField
            label="可见性"
            value={visibility}
            onChange={setVisibility}
            options={[
              ['public', '公开'],
              ['restricted', '受限'],
            ]}
          />
        )}
        <SnapshotEditor
          initial={
            resource.data?.revision?.snapshot ?? {
              ...emptySnapshot,
              title: create ? (params.get('title') ?? '').trim() : '',
            }
          }
          allowType={create}
          label={create ? '创建并发布' : '提交提案'}
          action={action}
          submit={async (snapshot, published) => {
            if (create) {
              const created = await action.write<Schema<'CreatedPage'>>('pages', {
                visibility,
                snapshot,
              })
              published()
              navigate('/pages/' + created.page.id)
            } else {
              const proposal = await action.write<Schema<'Proposal'>>(`pages/${id}/proposals`, {
                expected_head_revision_id: resource.data!.page.head_revision_id,
                snapshot,
              })
              published()
              navigate(`/pages/${id}/proposals/${proposal.id}`)
            }
          }}
        />
      </div>
    </section>
  )
}
export function HistoryScreen() {
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const left = params.get('from') ?? '',
    right = params.get('to') ?? ''
  const setLeft = (from: string) => setParams({ from, to: right }),
    setRight = (to: string) => setParams({ from: left, to })
  return (
    <>
      <PageHeader id={id} title="发布历史" />
      <Link to={`/pages/${id}`}>返回页面</Link>
      <Collection<Schema<'Revision'>> path={`pages/${id}/revisions`}>
        {(r) => (
          <div className="list-row" key={r.id}>
            <div>
              <h3>
                <Link to={`/pages/${id}/revisions/${r.id}`}>{r.snapshot.title}</Link>
              </h3>
              <small>
                <DateText value={r.created_at} /> · <span className="id">{r.id}</span>
              </small>
            </div>
            <div className="actions">
              <Button variant="outline" onClick={() => setLeft(r.id)}>
                设为对比起点
              </Button>
              <Button variant="outline" onClick={() => setRight(r.id)}>
                设为对比终点
              </Button>
            </div>
          </div>
        )}
      </Collection>
      {left && right && (
        <div className="compare">
          <RevisionCompare key={left} id={id} revision={left} label="起点" />
          <RevisionCompare key={right} id={id} revision={right} label="终点" />
        </div>
      )}
    </>
  )
}
function RevisionCompare({ id, revision, label }: { id: string; revision: string; label: string }) {
  const r = useResource<Schema<'Revision'>>(`pages/${id}/revisions/${revision}`)
  return (
    <section className="panel">
      <h2>{label}</h2>
      {r.data ? (
        <>
          <h3>{r.data.snapshot.title}</h3>
          <TagLinks tags={r.data.snapshot.tags} />
          <SnapshotView snapshot={r.data.snapshot} contentURL={r.data.content_url} />
        </>
      ) : (
        <Loading error={r.error} />
      )}
    </section>
  )
}
export function RevisionScreen() {
  const { id = '', revisionID = '' } = useParams(),
    resource = useResource<Schema<'Revision'>>(`pages/${id}/revisions/${revisionID}`)
  const page = useResource<Schema<'PageDetail'>>(`pages/${id}`),
    action = useAction(),
    navigate = useNavigate(),
    [kind, setKind] = useState('restore')
  if (!resource.data) return <Loading error={resource.error} />
  const revision = resource.data
  return (
    <>
      <PageHeader id={id} title={revision.snapshot.title} />
      <Link to={`/pages/${id}/history`}>返回历史</Link>
      <p className="metadata">
        <DateText value={revision.created_at} /> · 固定发布版本
      </p>
      <div className="panel">
        <SnapshotView snapshot={revision.snapshot} contentURL={revision.content_url} />
      </div>
      {page.data?.capabilities.includes('propose') && (
        <section className="panel stack">
          <h2>通过新提案回滚</h2>
          <p>保留历史与当前权限。恢复会取此快照，撤销会反向应用此发布的修改。</p>
          <SelectField
            label="回滚方式"
            value={kind}
            onChange={setKind}
            options={[
              ['restore', '恢复到此版本'],
              ['revert', '撤销此版本修改'],
            ]}
          />
          <Button
            disabled={action.busy || !page.data?.page.head_revision_id}
            onClick={() =>
              void action.run(async () => {
                const out = await action.write<Schema<'Proposal'>>(`pages/${id}/${kind}`, {
                  revision_id: revisionID,
                  expected_head_revision_id: page.data!.page.head_revision_id,
                })
                navigate(`/pages/${id}/proposals/${out.id}`)
              })
            }
          >
            创建回滚提案
          </Button>
          <ActionFeedback action={action} />
        </section>
      )}
    </>
  )
}
