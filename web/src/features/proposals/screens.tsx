import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import {
  Collection,
  DateText,
  Field,
  Loading,
  SectionTitle,
  SelectField,
  ActionFeedback,
} from '@/components/shared'
import { Button } from '@/components/ui/button'
import { SnapshotEditor } from '@/features/editing/editor'
import { SnapshotView } from '@/features/reading/markdown'

const stateLabel = { open: '待审核', merged: '已合并', closed: '已关闭' }
const decisionLabel = { approve: '批准', request_changes: '要求修改', comment: '评论' }

export function ProposalListScreen() {
  const { id } = useParams(),
    [state, setState] = useState('open')
  const path = `${id ? `pages/${id}/proposals` : 'proposals'}?state=${state}`
  return (
    <>
      <SectionTitle eyebrow="REVIEW TOGETHER" title="提案与审核" />
      <div className="toolbar">
        <SelectField
          label="提案状态"
          value={state}
          onChange={setState}
          options={[
            ['open', '待处理'],
            ['merged', '已合并'],
            ['closed', '已关闭'],
          ]}
        />
      </div>
      <Collection<Schema<'Proposal'>> key={path} path={path}>
        {(p) => (
          <div className="list-row" key={p.id}>
            <div>
              <h2>
                <Link to={`/proposals/${p.id}`}>{p.current_revision.snapshot.title}</Link>
              </h2>
              <p className="muted">
                {stateLabel[p.state]} · 第 {p.review_round} 轮审核 ·{' '}
                <DateText value={p.current_revision.created_at} />
              </p>
            </div>
            <Link to={`/proposals/${p.id}`}>查看提案 →</Link>
          </div>
        )}
      </Collection>
    </>
  )
}
export function ProposalScreen() {
  const { id = '' } = useParams(),
    [version, setVersion] = useState(0)
  const resource = useResource<Schema<'Proposal'>>(`proposals/${id}`, version)
  if (!resource.data) return <Loading error={resource.error} />
  return (
    <ProposalDetail
      key={`${id}:${version}`}
      proposal={resource.data}
      reload={() => setVersion((v) => v + 1)}
    />
  )
}
function ProposalDetail({
  proposal: p,
  reload,
}: {
  proposal: Schema<'Proposal'>
  reload: () => void
}) {
  const page = useResource<Schema<'PageDetail'>>(`pages/${p.page_id}`),
    diff = useResource<Schema<'Diff'>>(`proposals/${p.id}/diff`)
  const action = useAction(),
    editAction = useAction(),
    navigate = useNavigate(),
    [decision, setDecision] = useState('approve'),
    [body, setBody] = useState('')
  const pair = {
    expected_head_revision_id: page.data?.page.head_revision_id ?? null,
    expected_proposal_revision_id: p.current_revision.id,
  }
  const mutate = (op: string, input: unknown) =>
    action.run(async () => {
      const result = await action.write<Schema<'Revision'>>(`proposals/${p.id}/${op}`, input)
      if (op === 'merge') navigate(`/pages/${p.page_id}/revisions/${result.id}`)
      else reload()
    })
  return (
    <article>
      <SectionTitle
        eyebrow={`PROPOSAL · ${stateLabel[p.state]} · 第 ${p.review_round} 轮`}
        title={p.current_revision.snapshot.title}
      />
      <div className="actions">
        <Link to={`/pages/${p.page_id}`}>返回页面</Link>
        <Button variant="outline" onClick={reload}>
          重新读取（清空本地草稿）
        </Button>
      </div>
      <section className="panel">
        <h2>当前候选</h2>
        <SnapshotView
          snapshot={p.current_revision.snapshot}
          contentURL={
            p.current_revision.snapshot.type === 'media'
              ? `/api/v1/proposals/${p.id}/revisions/${p.current_revision.id}/content`
              : null
          }
        />
      </section>
      <section className="panel stack">
        <h2>变更对比</h2>
        {diff.data ? (
          <>
            {diff.data.stale && (
              <p className="warning">
                页面已有更新，请检查当前 head 并执行 rebase；旧批准可能已失效。
              </p>
            )}
            {diff.data.fields.length ? (
              diff.data.fields.map((f) => (
                <details key={f.path} open>
                  <summary>{f.path}</summary>
                  {f.unified_diff ? (
                    <pre className="diff">{f.unified_diff}</pre>
                  ) : (
                    <div className="compare">
                      <pre>{f.before ?? '（无）'}</pre>
                      <pre>{f.after ?? '（无）'}</pre>
                    </div>
                  )}
                </details>
              ))
            ) : (
              <p>没有内容差异。</p>
            )}
          </>
        ) : (
          <Loading error={diff.error} />
        )}
      </section>
      <section className="panel stack">
        <h2>审核与发布</h2>
        {!page.data && <Loading error={page.error} />}
        <p>审核绑定当前候选和页面版本；批准、修改要求与发布资格由服务端校验。</p>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void mutate('reviews', {
              ...pair,
              expected_review_round: p.review_round,
              decision,
              body,
            })
          }}
        >
          <SelectField
            label="审核意见"
            value={decision}
            onChange={setDecision}
            options={[
              ['approve', '批准'],
              ['request_changes', '要求修改'],
              ['comment', '仅评论'],
            ]}
          />
          <Field label="审核说明" value={body} onChange={setBody} multiline />
          <Button disabled={action.busy || !page.data || p.state !== 'open'}>提交审核</Button>
        </form>
        <div className="actions">
          <Button
            disabled={action.busy || !page.data || p.state !== 'open'}
            onClick={() => void mutate('merge', pair)}
          >
            合并并发布
          </Button>
          <Button
            variant="outline"
            disabled={action.busy || !page.data || p.state !== 'open'}
            onClick={() => void mutate('rebase', pair)}
          >
            基于当前发布重新合并
          </Button>
          {p.state !== 'merged' && (
            <Button
              variant="outline"
              disabled={action.busy}
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
        <ActionFeedback action={action} />
      </section>
      {p.state === 'open' && page.data && (
        <section className="panel stack">
          <h2>修订候选</h2>
          <p className="muted">
            提交修订后旧审核失效。冲突时草稿保持在此处，请勿直接覆盖其他发布。
          </p>
          <SnapshotEditor
            initial={p.current_revision.snapshot}
            action={editAction}
            label="提交候选修订"
            submit={async (snapshot) => {
              await editAction.write(`proposals/${p.id}/revisions`, { ...pair, snapshot })
              reload()
            }}
          />
        </section>
      )}
      <section className="panel">
        <h2>审核记录</h2>
        <Collection<Schema<'Review'>> path={`proposals/${p.id}/reviews`}>
          {(r) => (
            <div className="list-row" key={r.id}>
              <div>
                <strong>
                  {decisionLabel[r.decision]} · 第 {r.round} 轮
                </strong>
                <p>{r.body || '（未附说明）'}</p>
                <small className="id">
                  审核者 {r.actor_id} · <DateText value={r.created_at} />
                </small>
              </div>
            </div>
          )}
        </Collection>
      </section>
      <section className="panel">
        <h2>候选历史</h2>
        <Collection<Schema<'ProposalRevision'>> path={`proposals/${p.id}/revisions`}>
          {(r) => (
            <details key={r.id}>
              <summary>
                {r.snapshot.title} · <DateText value={r.created_at} />
              </summary>
              <small className="id">
                候选 {r.id} · 基础发布 {r.base_revision_id ?? '首次发布'}
              </small>
              <SnapshotView
                snapshot={r.snapshot}
                contentURL={
                  r.snapshot.type === 'media'
                    ? `/api/v1/proposals/${p.id}/revisions/${r.id}/content`
                    : null
                }
              />
            </details>
          )}
        </Collection>
      </section>
    </article>
  )
}
