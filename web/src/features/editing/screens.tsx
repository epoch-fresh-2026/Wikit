import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import { Loading, SectionTitle, SelectField } from '@/components/shared'
import { PageHeader } from '@/components/layout'
import { useAuth } from '@/features/account/auth'
import { SnapshotEditor, emptySnapshot } from './editor'

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
    <section className="editor-screen">
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
