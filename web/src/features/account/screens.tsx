import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { api, type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import { ActionFeedback, Field, Loading, SectionTitle } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from './auth'

export function LoginScreen({ register = false }: { register?: boolean }) {
  const [username, setUsername] = useState(''),
    [password, setPassword] = useState('')
  const [issued, setIssued] = useState('')
  const action = useAction(),
    auth = useAuth(),
    navigate = useNavigate()
  if (issued)
    return (
      <section className="auth panel">
        <SectionTitle title="账户已创建" />
        <p>请立即保存个人 Token。离开此页后无法再次读取。</p>
        <Secret value={issued} />
        <Button
          onClick={() => {
            setIssued('')
            navigate('/login')
          }}
        >
          已保存，前往登录
        </Button>
      </section>
    )
  return (
    <section className="auth panel">
      <SectionTitle
        title={register ? '注册' : '登录'}
        description={
          register ? '创建账户，参与知识的讨论与完善。' : '登录 Wikit，继续你的阅读与协作。'
        }
      />

      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault()
          void action.run(async () => {
            const value = await api.send<Schema<'Registration'> | Schema<'Session'>>(
              `auth/${register ? 'register' : 'login'}`,
              { username, password },
              'POST',
              undefined,
              false,
            )
            setPassword('')
            if ('token' in value) setIssued(value.token)
            else {
              auth.login(value)
              navigate('/')
            }
          })
        }}
      >
        <Field
          label="用户名"
          value={username}
          onChange={setUsername}
          autoComplete="username"
          required
        />
        <Field
          label="密码"
          autoComplete={register ? 'new-password' : 'current-password'}
          type="password"
          value={password}
          onChange={setPassword}
          required
          hint={register ? '12–128 个字符；用户名为 3–32 位字母、数字或下划线。' : undefined}
        />
        <Button disabled={action.busy}>{register ? '创建账户' : '登录'}</Button>
        <ActionFeedback action={action} />
      </form>
      <Link className="auth-switch" to={register ? '/login' : '/register'}>
        {register ? '已有账户？前往登录' : '还没有账户？立即注册'}
      </Link>
    </section>
  )
}
function Secret({ value }: { value: string }) {
  return (
    <div className="secret stack">
      <label htmlFor="issued-token">个人 Token（仅本次显示）</label>
      <Textarea
        id="issued-token"
        value={value}
        readOnly
        rows={3}
        spellCheck={false}
        autoComplete="off"
      />
      <small>此 Token 与账户共享权限。请勿发送到聊天、URL 或公开内容中。</small>
    </div>
  )
}
export function AccountScreen() {
  const auth = useAuth(),
    navigate = useNavigate(),
    action = useAction()
  const [revision, setRevision] = useState(0),
    [current, setCurrent] = useState(''),
    [next, setNext] = useState(''),
    [issued, setIssued] = useState('')
  const token = useResource<Schema<'TokenStatus'>>('auth/token', revision)
  if (!auth.account)
    return (
      <p>
        请先<Link to="/login">登录</Link>。
      </p>
    )
  const sensitive = (op: 'token' | 'token/revoke' | 'password') =>
    action.run(async () => {
      setIssued('')
      const out = await api.send<Schema<'Token'> | undefined>(`auth/${op}`, {
        current_password: current,
        ...(op === 'password' ? { new_password: next } : {}),
      })
      setCurrent('')
      setNext('')
      if (op === 'password') {
        auth.clear()
        navigate('/login')
        return
      }
      if (out) setIssued(out.token)
      setRevision((v) => v + 1)
      action.setNotice(
        op === 'token/revoke' ? 'Token 已撤销。' : '旧 Token 已失效，请保存新 Token。',
      )
    })
  return (
    <section className="narrow">
      <SectionTitle title={auth.account.display_name || auth.account.username} />
      <div className="panel stack">
        <p>
          用户名：{auth.account.username} · 角色：
          {{ member: '成员', moderator: '版主', admin: '管理员' }[auth.account.role]}
        </p>
        <small className="id">账户 ID：{auth.account.id}</small>
        <h2>个人 Token</h2>
        {token.data ? (
          <p>
            状态：{token.data.active ? '有效' : '已撤销'} · 代次 {token.data.generation}
          </p>
        ) : (
          <Loading error={token.error} />
        )}
        <p className="muted">每个账户最多一个有效 Token。轮换或撤销会让原 Token 立即失效。</p>
        <Field label="当前密码" value={current} type="password" onChange={setCurrent} required />
        <div className="actions">
          <Button disabled={action.busy || !current} onClick={() => void sensitive('token')}>
            轮换 Token
          </Button>
          <Button
            variant="outline"
            disabled={action.busy || !current}
            onClick={() => void sensitive('token/revoke')}
          >
            撤销 Token
          </Button>
        </div>
        {issued && (
          <>
            <Secret value={issued} />
            <Button variant="outline" onClick={() => setIssued('')}>
              已保存，清除明文
            </Button>
          </>
        )}
        <hr />
        <h2>修改密码</h2>
        <Field
          label="新密码"
          autoComplete="new-password"
          value={next}
          type="password"
          onChange={setNext}
          hint="修改后所有旧会话失效，需要重新登录。"
        />
        <Button
          disabled={action.busy || !current || !next}
          onClick={() => void sensitive('password')}
        >
          修改密码并退出
        </Button>
        <ActionFeedback action={action} />
      </div>
    </section>
  )
}
