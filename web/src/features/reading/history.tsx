import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import {
  Collection,
  DateText,
  Loading,
  SelectField,
  TagLinks,
  ActionFeedback,
} from '@/components/shared'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/layout'
import { SnapshotView } from './markdown'

export function HistoryScreen() {
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const left = params.get('from') ?? '',
    right = params.get('to') ?? ''
  const setLeft = (from: string) => setParams({ from, to: right }),
    setRight = (to: string) => setParams({ from: left, to })
  return (
    <>
      <PageHeader
        id={id}
        title="发布历史"
        description="每一次发布都有记录，选择两个版本查看变化。"
      />
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
