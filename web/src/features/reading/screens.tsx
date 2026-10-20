import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { BookOpen, ArrowUpRight, FilePlus2 } from 'lucide-react'
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
import { Card } from '@/components/ui/card'
import { useAuth } from '@/features/account/auth'
import { SnapshotEditor, emptySnapshot } from '@/features/editing/editor'
import { SnapshotView } from './markdown'

export function ExploreScreen() {
  const [params, setParams] = useSearchParams(),
    [q, setQ] = useState(params.get('q') ?? '')
  const tag = params.get('tag') ?? '',
    type = params.get('type') ?? '',
    search = params.get('q') ?? ''
  const path = search ? 'search?' + query({ q: search, tag }) : 'pages?' + query({ tag, type })
  return (
    <>
      <SectionTitle eyebrow="A SHARED PLACE FOR KNOWLEDGE" title="把知识，写在一起。">
        <Link className="button-link" to="/pages/new">
          <FilePlus2 size={16} aria-hidden /> 创建页面
        </Link>
      </SectionTitle>
      <p className="intro">从一个想法开始，通过讨论与审核，让知识不断生长。</p>
      <form
        className="searchbar"
        onSubmit={(e) => {
          e.preventDefault()
          setParams(query({ q: q.trim(), tag, type }))
        }}
      >
        <label className="sr-only" htmlFor="search">
          搜索知识
        </label>
        <input
          id="search"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="搜索页面、知识与灵感…"
        />
        <Button>搜索</Button>
      </form>
      <div className="browse-grid">
        <aside className="sidebar">
          <h2>探索知识</h2>
          <Link to="/" className={!tag && !search ? 'selected' : ''}>
            <BookOpen size={16} aria-hidden /> 全部页面
          </Link>
          <Link to="/proposals">
            待审提案 <ArrowUpRight size={15} aria-hidden />
          </Link>
          <hr />
          <h2>标签</h2>
          <Collection<Schema<'Tag'>> path="tags">
            {(t) => (
              <Link
                key={t.key}
                to={'/?' + query({ tag: t.key, q: search })}
                className={tag === t.key ? 'selected' : ''}
              >
                # {t.key}
              </Link>
            )}
          </Collection>
        </aside>
        <section aria-label="页面结果">
          <div className="results-heading">
            <h2>{search ? `“${search}”的搜索结果` : tag ? `标签 · ${tag}` : '发现与阅读'}</h2>
            {!search && (
              <SelectField
                label="页面类型筛选"
                value={type}
                onChange={(v) => setParams(query({ tag, type: v }))}
                options={[
                  ['', '全部类型'],
                  ['document', '文档'],
                  ['media', '媒体'],
                ]}
              />
            )}
          </div>
          {search ? (
            <Collection<Schema<'SearchHit'>>
              key={path}
              path={path}
              empty="未找到匹配页面，请尝试其他关键词。"
            >
              {(hit) => (
                <Card className="page-card" key={hit.page_id}>
                  <div className="kicker">{hit.type === 'document' ? '文档' : '媒体'}</div>
                  <h3>
                    <Link to={'/pages/' + hit.page_id}>{hit.title}</Link>
                  </h3>
                  <p>{hit.snippet}</p>
                  <TagLinks tags={hit.tags} />
                </Card>
              )}
            </Collection>
          ) : (
            <Collection<Schema<'PageDetail'>>
              key={path}
              path={path}
              empty="这里还没有已发布页面。创建第一个提案，一起完善知识。"
            >
              {(item) => (
                <Card className="page-card" key={item.page.id}>
                  <div className="kicker">
                    {item.page.type === 'document' ? '文档' : '媒体'} ·{' '}
                    {item.page.visibility === 'public' ? '公开' : '受限'}
                  </div>
                  <h3>
                    <Link to={'/pages/' + item.page.id}>
                      {item.revision?.snapshot.title ?? '尚未发布'}
                    </Link>
                  </h3>
                  {item.revision && (
                    <>
                      <p className="excerpt">
                        {item.revision.snapshot.type === 'document'
                          ? item.revision.snapshot.markdown.slice(0, 160)
                          : item.revision.snapshot.description.slice(0, 160)}
                      </p>
                      <TagLinks tags={item.revision.snapshot.tags} />
                      <small>
                        更新于 <DateText value={item.revision.created_at} />
                      </small>
                    </>
                  )}
                </Card>
              )}
            </Collection>
          )}
        </section>
      </div>
    </>
  )
}

export function PageScreen() {
  const { id = '' } = useParams(),
    resource = useResource<Schema<'PageDetail'>>(`pages/${id}`)
  if (!resource.data) return <Loading error={resource.error} />
  const { page, revision } = resource.data
  return (
    <article>
      <SectionTitle
        eyebrow={page.type === 'document' ? 'DOCUMENT' : 'MEDIA'}
        title={revision?.snapshot.title ?? '页面尚未发布'}
      />
      <nav className="tabs" aria-label="页面操作">
        <Link to={`/pages/${id}/edit`}>提出修改</Link>
        <Link to={`/pages/${id}/history`}>发布历史</Link>
        <Link to={`/pages/${id}/proposals`}>提案</Link>
        <Link to={`/pages/${id}/issues`}>讨论</Link>
        <Link to={`/pages/${id}/policy`}>权限与治理</Link>
      </nav>
      {revision ? (
        <>
          <TagLinks tags={revision.snapshot.tags} />
          <p className="metadata">
            {page.visibility === 'restricted' ? '受限页面' : '公开页面'} ·{' '}
            <DateText value={revision.created_at} />
          </p>
          <div className="panel reading">
            <SnapshotView snapshot={revision.snapshot} contentURL={revision.content_url} />
          </div>
        </>
      ) : (
        <div className="empty">
          创建提案审核合并后，页面会出现在目录与搜索中。
          <Link to={`/pages/${id}/proposals`}>查看创建提案</Link>
        </div>
      )}
    </article>
  )
}
export function EditPageScreen({ create = false }: { create?: boolean }) {
  const { id = '' } = useParams(),
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
  if (!create && !resource.data) return <Loading error={resource.error} />
  if (!create && !resource.data?.revision)
    return (
      <p>
        页面尚未发布，请从<Link to={`/pages/${id}/proposals`}>创建提案</Link>继续编辑。
      </p>
    )
  return (
    <section>
      <SectionTitle eyebrow="CONTRIBUTE" title={create ? '创建知识页面' : '提出页面修改'} />
      <p className="muted">内容先保存为提案，经有效审核与合并后发布。</p>
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
          initial={resource.data?.revision?.snapshot ?? emptySnapshot}
          allowType={create}
          action={action}
          submit={async (snapshot) => {
            if (create) {
              const created = await action.write<Schema<'CreatedPage'>>('pages', {
                visibility,
                snapshot,
              })
              navigate('/proposals/' + created.proposal.id)
            } else {
              const proposal = await action.write<Schema<'Proposal'>>(`pages/${id}/proposals`, {
                expected_head_revision_id: resource.data!.page.head_revision_id,
                snapshot,
              })
              navigate('/proposals/' + proposal.id)
            }
          }}
        />
      </div>
    </section>
  )
}
export function HistoryScreen() {
  const { id = '' } = useParams()
  const [left, setLeft] = useState(''),
    [right, setRight] = useState('')
  return (
    <>
      <SectionTitle title="发布历史与版本对比" />
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
      <SectionTitle eyebrow="IMMUTABLE REVISION" title={revision.snapshot.title} />
      <Link to={`/pages/${id}/history`}>返回历史</Link>
      <p className="metadata">
        <DateText value={revision.created_at} /> · 固定发布版本
      </p>
      <div className="panel">
        <SnapshotView snapshot={revision.snapshot} contentURL={revision.content_url} />
      </div>
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
              navigate('/proposals/' + out.id)
            })
          }
        >
          创建回滚提案
        </Button>
        <ActionFeedback action={action} />
      </section>
    </>
  )
}
