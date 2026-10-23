import { useState } from 'react'
import { Link, useParams } from 'react-router'
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
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from '@/components/ui/table'
import { Checkbox } from '@/components/ui/checkbox'
import { PageHeader } from '@/components/layout'
import { Moderation } from './moderation'

export function PolicyScreen() {
  const { id = '' } = useParams(),
    [revision, setRevision] = useState(0),
    resource = useResource<Schema<'Policy'>>(`pages/${id}/policy`, revision)
  if (!resource.data) return <Loading error={resource.error} />
  return (
    <PolicyEditor
      key={revision}
      id={id}
      policy={resource.data}
      reload={() => setRevision((v) => v + 1)}
    />
  )
}
function PolicyEditor({
  id,
  policy,
  reload,
}: {
  id: string
  policy: Schema<'Policy'>
  reload: () => void
}) {
  const [visibility, setVisibility] = useState(policy.visibility),
    [grants, setGrants] = useState(policy.grants),
    [moderationMode, setMode] = useState('hide')
  const action = useAction()
  const updateGrant = (index: number, grant: Schema<'Grant'>) =>
    setGrants((rows) => rows.map((g, i) => (i === index ? grant : g)))
  return (
    <>
      <PageHeader id={id} title="页面管理" />
      <Link to={`/pages/${id}`}>返回页面</Link>
      <section className="panel stack">
        <h2>访问策略</h2>
        <p className="muted">设置哪些人可以阅读、讨论或修改此页面。保存后立即生效。</p>
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault()
            void action.run(async () => {
              await action.write(
                `pages/${id}/policy`,
                { visibility, grants, expected_policy_version: policy.policy_version },
                'PUT',
              )
              reload()
            })
          }}
        >
          <SelectField
            label="页面可见性"
            value={visibility}
            onChange={(v) => setVisibility(v as typeof visibility)}
            options={
              id === '1'
                ? [['public', '公开']]
                : [
                    ['public', '公开'],
                    ['restricted', '受限'],
                  ]
            }
          />
          {grants.map((grant, i) => (
            <fieldset className="grant panel" key={i}>
              <legend>授权 {i + 1}</legend>
              <SelectField
                label={`授权 ${i + 1} 主体类型`}
                value={grant.subject_type}
                onChange={(v) => updateGrant(i, { ...grant, subject_type: v as 'user' | 'group' })}
                options={[
                  ['user', '账户'],
                  ['group', '用户组'],
                ]}
              />
              <Field
                label={`授权 ${i + 1} 主体 ID`}
                hint={grant.subject_type === 'user' ? '填写账户数字 ID' : '填写用户组 UUID'}
                value={grant.subject_id}
                onChange={(v) => updateGrant(i, { ...grant, subject_id: v })}
                required
              />
              <div className="actions">
                {(['read', 'propose', 'discuss', 'merge', 'manage'] as const).map((capability) => (
                  <label key={capability}>
                    <Checkbox
                      checked={grant.capabilities.includes(capability)}
                      onCheckedChange={(checked) =>
                        updateGrant(i, {
                          ...grant,
                          capabilities:
                            checked === true
                              ? [...grant.capabilities, capability]
                              : grant.capabilities.filter((c) => c !== capability),
                        })
                      }
                    />{' '}
                    {
                      {
                        read: '阅读',
                        propose: '提案',
                        discuss: '讨论',
                        merge: '合并',
                        manage: '管理',
                      }[capability]
                    }
                  </label>
                ))}
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => setGrants((rows) => rows.filter((_, j) => i !== j))}
              >
                移除此授权
              </Button>
            </fieldset>
          ))}
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              setGrants((rows) => [
                ...rows,
                { subject_type: 'user', subject_id: '', capabilities: ['read'] },
              ])
            }
          >
            添加授权
          </Button>
          <Button disabled={action.busy}>保存页面权限</Button>
          <ActionFeedback action={action} />
        </form>
        <Button variant="outline" onClick={reload}>
          重新读取策略（清空本地修改）
        </Button>
      </section>
      {id !== '1' && (
        <section className="panel stack">
          <h2>页面治理</h2>
          <p>隐藏会阻止普通读取，恢复保留既有历史及权限。请选择操作并填写理由。</p>
          <SelectField
            label="页面治理动作"
            value={moderationMode}
            onChange={setMode}
            options={[
              ['hide', '隐藏页面'],
              ['restore', '恢复页面'],
            ]}
          />
          <Moderation
            path={`pages/${id}/moderation`}
            hidden={moderationMode === 'restore'}
            version={policy.policy_version}
            done={reload}
          />
        </section>
      )}
    </>
  )
}

export function AdminScreen() {
  const { section: tab = 'accounts' } = useParams()
  const [revision, setRevision] = useState(0),
    [name, setName] = useState('')
  const action = useAction(),
    reload = () => setRevision((v) => v + 1)
  return (
    <>
      <SectionTitle
        title={{ accounts: '账户', groups: '用户组', audit: '审计' }[tab] ?? '未找到此页面'}
      />
      <div className="toolbar">
        <Button variant="outline" onClick={reload}>
          重新读取
        </Button>
      </div>
      {tab === 'accounts' && (
        <Collection<Schema<'AccountAdmin'>> key={revision} path="admin/accounts">
          {(item) => <AccountRow key={item.account.id} item={item} done={reload} />}
        </Collection>
      )}
      {tab === 'groups' && (
        <>
          <Collection<Schema<'Group'>> key={revision} path="admin/groups">
            {(group) => <GroupRow key={group.id} group={group} done={reload} />}
          </Collection>
          <section className="panel">
            <h2>创建用户组</h2>
            <form
              className="stack"
              onSubmit={(e) => {
                e.preventDefault()
                void action.run(async () => {
                  await action.write('admin/groups', { name, create_page: false })
                  setName('')
                  reload()
                })
              }}
            >
              <Field label="新用户组名称" value={name} onChange={setName} required />
              <Button disabled={action.busy}>创建用户组</Button>
              <ActionFeedback action={action} />
            </form>
          </section>
        </>
      )}
      {tab === 'audit' && (
        <Collection<Schema<'AuditEvent'>>
          key={revision}
          path="admin/audit"
          renderItems={(rows) => (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>操作</TableHead>
                  <TableHead>对象</TableHead>
                  <TableHead>账户</TableHead>
                  <TableHead>时间</TableHead>
                  <TableHead>请求</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>{rows}</TableBody>
            </Table>
          )}
        >
          {(event) => (
            <TableRow key={event.id}>
              <TableCell>{event.action}</TableCell>
              <TableCell>{event.target_id ?? '—'}</TableCell>
              <TableCell>{event.actor_id ?? '系统'}</TableCell>
              <TableCell>
                <DateText value={event.created_at} />
              </TableCell>
              <TableCell>{event.request_id}</TableCell>
            </TableRow>
          )}
        </Collection>
      )}
    </>
  )
}
function AccountRow({ item, done }: { item: Schema<'AccountAdmin'>; done: () => void }) {
  const [role, setRole] = useState(item.account.role),
    [status, setStatus] = useState(item.account.status),
    [createPage, setCreatePage] = useState(item.account.create_page),
    action = useAction()
  return (
    <section className="panel">
      <h2>{item.account.username}</h2>
      <p className="id">{item.account.id}</p>
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault()
          void action.run(async () => {
            await action.write(
              `admin/accounts/${item.account.id}`,
              { role, status, create_page: createPage, expected_auth_version: item.auth_version },
              'PATCH',
            )
            done()
          })
        }}
      >
        <div className="compare">
          <SelectField
            label={`${item.account.username} 角色`}
            value={role}
            onChange={(v) => setRole(v as typeof role)}
            options={[
              ['member', '成员'],
              ['moderator', '版主'],
              ['admin', '管理员'],
            ]}
          />
          <SelectField
            label={`${item.account.username} 状态`}
            value={status}
            onChange={(v) => setStatus(v as typeof status)}
            options={[
              ['active', '启用'],
              ['disabled', '禁用'],
            ]}
          />
        </div>
        <label className="check-label">
          <Checkbox checked={createPage} onCheckedChange={(v) => setCreatePage(v === true)} />
          创建页面
        </label>
        <Button variant="outline" disabled={action.busy}>
          保存账户变更
        </Button>
        <ActionFeedback action={action} />
      </form>
    </section>
  )
}
function GroupRow({ group, done }: { group: Schema<'Group'>; done: () => void }) {
  const [name, setName] = useState(group.name),
    [members, setMembers] = useState(group.member_ids),
    [createPage, setCreatePage] = useState(group.create_page),
    action = useAction()
  return (
    <section className="panel">
      <h2>{group.name}</h2>
      <p className="id">{group.id}</p>
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault()
          void action.run(async () => {
            await action.write(
              `admin/groups/${group.id}`,
              {
                name,
                member_ids: members,
                create_page: createPage,
                expected_version: group.version,
              },
              'PUT',
            )
            done()
          })
        }}
      >
        <Field label={`${group.name} 名称`} value={name} onChange={setName} required />
        <fieldset className="stack">
          <legend>成员</legend>
          <Collection<Schema<'AccountAdmin'>> path="admin/accounts">
            {({ account }) => (
              <label className="check-label" key={account.id}>
                <Checkbox
                  checked={members.includes(account.id)}
                  onCheckedChange={(v) =>
                    setMembers((current) =>
                      v === true
                        ? [...current, account.id]
                        : current.filter((id) => id !== account.id),
                    )
                  }
                />
                <span className="min-w-0">
                  {account.display_name || account.username}{' '}
                  <span className="muted">{account.username}</span>
                </span>
              </label>
            )}
          </Collection>
        </fieldset>
        <label className="check-label">
          <Checkbox checked={createPage} onCheckedChange={(v) => setCreatePage(v === true)} />
          组成员可创建页面
        </label>
        <Button variant="outline" disabled={action.busy}>
          保存用户组
        </Button>
        <ActionFeedback action={action} />
      </form>
    </section>
  )
}
