import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import {
  ActionFeedback,
  Collection,
  DateText,
  Field,
  Loading,
  SectionTitle,
  SelectField,
} from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/features/reading/markdown'
import { Moderation } from '@/features/governance/moderation'

export function IssueListScreen() {
  const { id = '' } = useParams(),
    navigate = useNavigate(),
    action = useAction()
  const [title, setTitle] = useState(''),
    [body, setBody] = useState(''),
    [state, setState] = useState('open'),
    [hidden, setHidden] = useState(false)
  const path = `pages/${id}/issues?state=${state}&include_hidden=${hidden}`
  return (
    <>
      <SectionTitle title="页面讨论" />
      <Link to={`/pages/${id}`}>返回页面</Link>
      <div className="toolbar">
        <SelectField
          label="议题状态"
          value={state}
          onChange={setState}
          options={[
            ['open', '进行中'],
            ['closed', '已关闭'],
          ]}
        />
        <label>
          <input type="checkbox" checked={hidden} onChange={(e) => setHidden(e.target.checked)} />{' '}
          治理读取（包含隐藏内容）
        </label>
      </div>
      <Collection<Schema<'Issue'>> key={path} path={path}>
        {(i) => (
          <div className="list-row" key={i.id}>
            <div>
              <h2>
                <Link to={`/issues/${i.id}${hidden ? '?governance=1' : ''}`}>{i.title}</Link>
              </h2>
              <small>
                {i.state}
                {i.hidden ? ' · 已隐藏' : ''} · <DateText value={i.created_at} />
              </small>
            </div>
          </div>
        )}
      </Collection>
      <section className="panel">
        <h2>发起议题</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void action.run(async () => {
              const issue = await action.write<Schema<'Issue'>>(`pages/${id}/issues`, {
                title,
                body,
              })
              navigate(`/issues/${issue.id}`)
            })
          }}
        >
          <Field label="议题标题" value={title} onChange={setTitle} required />
          <Field label="议题内容" value={body} onChange={setBody} multiline />
          <Button disabled={action.busy}>创建议题</Button>
          <ActionFeedback action={action} />
        </form>
      </section>
    </>
  )
}
export function IssueScreen({ governance = false }: { governance?: boolean }) {
  const { id = '' } = useParams(),
    [revision, setRevision] = useState(0),
    resource = useResource<Schema<'Issue'>>(`issues/${id}?include_hidden=${governance}`, revision)
  if (!resource.data) return <Loading error={resource.error} />
  return (
    <IssueDetail
      key={`${id}:${revision}`}
      issue={resource.data}
      governance={governance}
      reload={() => setRevision((v) => v + 1)}
    />
  )
}
function IssueDetail({
  issue,
  governance,
  reload,
}: {
  issue: Schema<'Issue'>
  governance: boolean
  reload: () => void
}) {
  const action = useAction(),
    commentAction = useAction(),
    [body, setBody] = useState(''),
    [proposal, setProposal] = useState('')
  return (
    <>
      <SectionTitle
        eyebrow={`DISCUSSION · ${issue.state === 'open' ? '进行中' : '已关闭'}${issue.hidden ? ' · 已隐藏' : ''}`}
        title={issue.title}
      />
      <Link to={`/pages/${issue.page_id}/issues`}>返回页面讨论</Link>
      <div className="panel">
        <Markdown text={issue.body} />
        <small>
          <DateText value={issue.created_at} />
        </small>
      </div>
      <section className="panel stack">
        <h2>议题操作</h2>
        <Button
          variant="outline"
          disabled={action.busy}
          onClick={() =>
            void action.run(async () => {
              await action.write(`issues/${issue.id}/state`, {
                state: issue.state === 'open' ? 'closed' : 'open',
                expected_version: issue.version,
              })
              reload()
            })
          }
        >
          {issue.state === 'open' ? '关闭议题' : '重开议题'}
        </Button>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void action.run(async () => {
              await action.write(`issues/${issue.id}/proposals`, {
                proposal_id: proposal,
                expected_version: issue.version,
              })
              reload()
            })
          }}
        >
          <Field label="关联同一页面的提案 ID" value={proposal} onChange={setProposal} required />
          <Button variant="outline" disabled={action.busy}>
            关联提案
          </Button>
        </form>
        <div className="stack">
          {issue.proposal_ids.map((id) => (
            <Link className="id" key={id} to={`/proposals/${id}`}>
              关联提案 {id}
            </Link>
          ))}
        </div>
        <ActionFeedback action={action} />
        <Moderation
          path={`issues/${issue.id}/moderation`}
          hidden={issue.hidden}
          version={issue.version}
          done={reload}
        />
      </section>
      <section className="panel">
        <h2>评论</h2>
        <Collection<Schema<'Comment'>>
          path={`issues/${issue.id}/comments?include_hidden=${governance}`}
        >
          {(c) => (
            <div className="comment" key={c.id}>
              <small className="id">
                {c.author_id} · <DateText value={c.created_at} />
                {c.hidden ? ' · 已隐藏' : ''}
              </small>
              <Markdown text={c.body} />
              {governance && (
                <Moderation
                  path={`comments/${c.id}/moderation`}
                  hidden={c.hidden}
                  version={c.version}
                  done={reload}
                />
              )}
            </div>
          )}
        </Collection>
      </section>
      <section className="panel">
        <h2>添加不可变评论</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void commentAction.run(async () => {
              await commentAction.write(`issues/${issue.id}/comments`, { body })
              setBody('')
              reload()
            })
          }}
        >
          <Field label="评论内容" value={body} onChange={setBody} multiline required />
          <Button disabled={commentAction.busy}>发表评论</Button>
          <ActionFeedback action={commentAction} />
        </form>
        <p className="muted">评论提交后不可编辑，请在提交前检查。</p>
      </section>
    </>
  )
}
