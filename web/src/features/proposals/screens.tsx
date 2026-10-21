import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import {
  ActionFeedback,
  Collection,
  DateText,
  Loading,
  SectionTitle,
  SelectField,
  TagLinks,
} from '@/components/shared'
import { PageNavigation } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SnapshotEditor } from '@/features/editing/editor'
import { SnapshotView } from '@/features/reading/markdown'
import { useAuth } from '@/features/account/auth'

const stateLabel = { open: '待处理', merged: '已合并', closed: '已关闭' }
export function ProposalListScreen() {
  const { id } = useParams(),
    [params, setParams] = useSearchParams(),
    state = params.get('state') ?? 'open'
  const path = `${id ? `pages/${id}/proposals` : 'proposals'}?state=${state}`
  return (
    <>
      {id && <PageNavigation id={id} />}
      <SectionTitle title={id ? '页面提案' : '全站提案'} />
      <SelectField
        label="提案状态"
        value={state}
        onChange={(v) => setParams({ state: v })}
        options={Object.entries(stateLabel)}
      />
      <Collection<Schema<'Proposal'>> key={path} path={path}>
        {(p) => (
          <div className="list-row" key={p.id}>
            <div>
              <Link to={`/pages/${p.page_id}/proposals/${p.id}`}>
                {p.current_revision.snapshot.title}
              </Link>
              <p className="muted">
                页面 {p.page_id} · <DateText value={p.current_revision.created_at} />
              </p>
            </div>
            <Badge variant="outline">{stateLabel[p.state]}</Badge>
          </div>
        )}
      </Collection>
    </>
  )
}
export function ProposalScreen({ edit = false }: { edit?: boolean }) {
  const { id = '', pageId = '' } = useParams(),
    [version, setVersion] = useState(0)
  const resource = useResource<Schema<'Proposal'>>(`proposals/${id}`, version)
  if (!resource.data) return <Loading error={resource.error} />
  if (resource.data.page_id !== pageId) return <p role="alert">未找到此提案。</p>
  return (
    <ProposalDetail
      key={`${id}:${version}`}
      proposal={resource.data}
      edit={edit}
      reload={() => setVersion((v) => v + 1)}
    />
  )
}
function ProposalDetail({
  proposal: p,
  edit,
  reload,
}: {
  proposal: Schema<'Proposal'>
  edit: boolean
  reload: () => void
}) {
  const page = useResource<Schema<'PageDetail'>>(`pages/${p.page_id}`),
    diff = useResource<Schema<'Diff'>>(`proposals/${p.id}/diff`)
  const stale =
    !!page.data && p.current_revision.base_revision_id !== page.data.page.head_revision_id
  const baseRevision = useResource<Schema<'Revision'>>(
    edit && stale && p.current_revision.base_revision_id
      ? `pages/${p.page_id}/revisions/${p.current_revision.base_revision_id}`
      : null,
  )
  const [resolved, setResolved] = useState(false)
  const auth = useAuth(),
    action = useAction(),
    navigate = useNavigate(),
    [params, setParams] = useSearchParams()
  const base = `/pages/${p.page_id}/proposals/${p.id}`,
    caps = page.data?.capabilities ?? [],
    view = params.get('view') ?? 'diff'
  const canEdit =
    (p.author_id === auth.account?.id || caps.includes('manage')) &&
    caps.includes('propose') &&
    p.state === 'open'
  const pair = {
    expected_head_revision_id: page.data?.page.head_revision_id ?? null,
    expected_proposal_revision_id: p.current_revision.id,
  }
  const mutate = (op: string, body: unknown) =>
    action.run(async () => {
      await action.write(`proposals/${p.id}/${op}`, body)
      if (op === 'merge') navigate(`/pages/${p.page_id}`)
      else reload()
    })
  return (
    <>
      <PageNavigation
        id={p.page_id}
        title={page.data?.revision?.snapshot.title}
        capabilities={caps}
      />
      <SectionTitle title={edit ? '编辑提案' : p.current_revision.snapshot.title}>
        <Badge variant="outline">{stateLabel[p.state]}</Badge>
      </SectionTitle>
      {!page.data ? (
        <Loading error={page.error} />
      ) : edit ? (
        canEdit ? (
          <>
            {stale && (
              <section className="panel stack">
                <h2>处理过期提案</h2>
                <p>
                  页面已有新版本。请对照基础与当前发布内容编辑候选，保留需要的修改；保存后提案将以当前发布版本为基础。
                </p>
                <div className="compare">
                  <section aria-label="基础版本">
                    <h3>基础版本</h3>
                    {baseRevision.data ? (
                      <>
                        <h4>{baseRevision.data.snapshot.title}</h4>
                        <TagLinks tags={baseRevision.data.snapshot.tags} />
                        <SnapshotView
                          snapshot={baseRevision.data.snapshot}
                          contentURL={baseRevision.data.content_url}
                        />
                      </>
                    ) : (
                      <Loading error={baseRevision.error} />
                    )}
                  </section>
                  <section aria-label="当前发布版本">
                    <h3>当前发布版本</h3>
                    {page.data.revision && (
                      <>
                        <h4>{page.data.revision.snapshot.title}</h4>
                        <TagLinks tags={page.data.revision.snapshot.tags} />
                        <SnapshotView
                          snapshot={page.data.revision.snapshot}
                          contentURL={page.data.revision.content_url}
                        />
                      </>
                    )}
                  </section>
                </div>
                <label className="check-label">
                  <Checkbox
                    checked={resolved}
                    disabled={!baseRevision.data || !page.data.revision}
                    onCheckedChange={(v) => setResolved(v === true)}
                  />
                  已结合当前发布内容解决冲突
                </label>
              </section>
            )}
            <SnapshotEditor
              initial={p.current_revision.snapshot}
              action={action}
              label="保存修订"
              submitDisabled={stale && (!resolved || !baseRevision.data || !page.data.revision)}
              submit={async (snapshot, published) => {
                await action.write(`proposals/${p.id}/revisions`, { ...pair, snapshot })
                published()
                navigate(base)
              }}
            />
          </>
        ) : (
          <p role="alert">没有编辑此提案的权限。</p>
        )
      ) : (
        <>
          <div className="toolbar">
            {canEdit && (
              <Button asChild variant="outline">
                <Link to={base + '/edit'}>编辑提案</Link>
              </Button>
            )}
            {caps.includes('merge') && p.state === 'open' && (
              <Button
                disabled={
                  view !== 'diff' ||
                  action.busy ||
                  !diff.data ||
                  diff.data.stale ||
                  diff.data.proposal_revision_id !== p.current_revision.id ||
                  diff.data.head_revision_id !== pair.expected_head_revision_id
                }
                title={view !== 'diff' ? '请先查看变更对比' : undefined}
                onClick={() => void mutate('merge', pair)}
              >
                合并
              </Button>
            )}
            {(p.author_id === auth.account?.id || caps.includes('manage')) &&
              p.state !== 'merged' && (
                <Button
                  variant="ghost"
                  disabled={action.busy || (p.state === 'closed' && !caps.includes('propose'))}
                  onClick={() =>
                    void mutate('state', {
                      state: p.state === 'open' ? 'closed' : 'open',
                      expected_proposal_version: p.version,
                    })
                  }
                >
                  {p.state === 'open' ? '关闭提案' : '重开提案'}
                </Button>
              )}
          </div>
          {diff.data?.stale && (
            <div className="conflict">
              <p>页面已有新版本。请更新提案基础并检查新的差异。</p>
              {canEdit && (
                <Button
                  variant="outline"
                  disabled={action.busy}
                  onClick={() => void mutate('rebase', pair)}
                >
                  更新基础版本
                </Button>
              )}
            </div>
          )}
          <Tabs value={view} onValueChange={(view) => setParams({ view })}>
            <TabsList>
              <TabsTrigger value="diff">变更对比</TabsTrigger>
              <TabsTrigger value="preview">候选预览</TabsTrigger>
              <TabsTrigger value="history">候选历史</TabsTrigger>
            </TabsList>
            <TabsContent value="diff">
              {diff.data ? (
                diff.data.fields.length ? (
                  diff.data.fields.map((f) => (
                    <section className="diff-field" key={f.path}>
                      <h2>
                        {{
                          '/title': '标题',
                          '/tags': '标签',
                          '/markdown': '正文',
                          '/description': '媒体描述',
                          '/blob_id': '媒体原件',
                        }[f.path] ?? f.path}
                      </h2>
                      {f.unified_diff ? (
                        <pre className="diff">{f.unified_diff}</pre>
                      ) : (
                        <div className="compare">
                          <div>
                            <h3>修改前</h3>
                            <pre>{f.before ?? '无'}</pre>
                          </div>
                          <div>
                            <h3>修改后</h3>
                            <pre>{f.after ?? '无'}</pre>
                          </div>
                        </div>
                      )}
                    </section>
                  ))
                ) : (
                  <p className="empty">没有内容差异。</p>
                )
              ) : (
                <Loading error={diff.error} />
              )}
            </TabsContent>
            <TabsContent value="preview">
              <SnapshotView
                snapshot={p.current_revision.snapshot}
                contentURL={
                  p.current_revision.snapshot.type === 'media'
                    ? `/api/v1/proposals/${p.id}/revisions/${p.current_revision.id}/content`
                    : null
                }
              />
            </TabsContent>
            <TabsContent value="history">
              <Collection<Schema<'ProposalRevision'>> path={`proposals/${p.id}/revisions`}>
                {(r) => (
                  <section className="directory-row" key={r.id}>
                    <DateText value={r.created_at} />
                    <SnapshotView
                      snapshot={r.snapshot}
                      contentURL={
                        r.snapshot.type === 'media'
                          ? `/api/v1/proposals/${p.id}/revisions/${r.id}/content`
                          : null
                      }
                    />
                  </section>
                )}
              </Collection>
            </TabsContent>
          </Tabs>
          <ActionFeedback action={action} />
          {action.error && (
            <Button variant="outline" onClick={reload}>
              重新读取
            </Button>
          )}
        </>
      )}
    </>
  )
}
