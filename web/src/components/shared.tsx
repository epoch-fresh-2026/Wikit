import { useId, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { ArrowRight, AlertCircle } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { useResource, type useAction } from '@/lib/hooks'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Textarea } from './ui/textarea'
import { Label } from './ui/label'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from './ui/select'
import { Badge } from './ui/badge'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { Skeleton } from './ui/skeleton'
import { useEffect } from 'react'

export function Field({
  label,
  value,
  onChange,
  multiline = false,
  type = 'text',
  required = false,
  children,
  hint,
  autoComplete,
}: {
  label: string
  value?: string
  onChange?: (v: string) => void
  multiline?: boolean
  type?: string
  required?: boolean
  children?: ReactNode
  hint?: string
  autoComplete?: string
}) {
  const id = useId()
  return (
    <div className="field">
      <Label htmlFor={id}>{label}</Label>
      {children ??
        (multiline ? (
          <Textarea
            id={id}
            value={value}
            onChange={(e) => onChange?.(e.target.value)}
            required={required}
            aria-describedby={hint ? `${id}-hint` : undefined}
            rows={8}
          />
        ) : (
          <Input
            id={id}
            type={type}
            value={value}
            onChange={(e) => onChange?.(e.target.value)}
            required={required}
            autoComplete={autoComplete ?? (type === 'password' ? 'current-password' : undefined)}
            aria-describedby={hint ? `${id}-hint` : undefined}
          />
        ))}
      {hint && <small id={`${id}-hint`}>{hint}</small>}
    </div>
  )
}
export function SelectField({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: [string, string][]
}) {
  const id = useId()
  return (
    <div className="field">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value || '__all__'} onValueChange={(v) => onChange(v === '__all__' ? '' : v)}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([v, text]) => (
            <SelectItem key={v} value={v || '__all__'}>
              {text}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
export function ErrorNotice({ error }: { error?: ApiError }) {
  if (!error) return null
  return (
    <Alert variant="destructive" className="error">
      <AlertCircle aria-hidden />
      <AlertTitle className="min-w-0 line-clamp-none [overflow-wrap:anywhere]">
        {error.message}
      </AlertTitle>
      <AlertDescription className="min-w-0 [overflow-wrap:anywhere]">
        {error.requestID && <small>请求编号：{error.requestID}</small>}
        {error.code === 'unauthenticated' && <Link to="/login">重新登录</Link>}
        {error.conflicts?.map((c, i) => (
          <section className="conflict w-full min-w-0" key={i}>
            <strong>{c.path}</strong>
            <div className="compare">
              <div>
                基础<pre>{c.base ?? '（无）'}</pre>
              </div>
              <div>
                当前发布<pre>{c.ours ?? '（无）'}</pre>
              </div>
              <div>
                待合入内容<pre>{c.theirs ?? '（无）'}</pre>
              </div>
            </div>
          </section>
        ))}
      </AlertDescription>
    </Alert>
  )
}
export function ActionFeedback({ action }: { action: ReturnType<typeof useAction> }) {
  return (
    <>
      <ErrorNotice error={action.error} />
      <p role="status" className="status">
        {action.busy ? '正在提交…' : action.notice}
      </p>
    </>
  )
}
export function Loading({ error }: { error?: ApiError }) {
  return error ? (
    <ErrorNotice error={error} />
  ) : (
    <div role="status" className="muted">
      <Skeleton className="h-5 w-40" />
      <span className="sr-only">正在读取…</span>
    </div>
  )
}
export function SectionTitle({ title, children }: { title: string; children?: ReactNode }) {
  useEffect(() => {
    document.title = title + ' · Wikit'
  }, [title])
  return (
    <header className="section-title">
      <div>
        <h1>{title}</h1>
      </div>
      {children}
    </header>
  )
}
export function TagLinks({ tags }: { tags: string[] }) {
  return (
    <div className="tags">
      {tags.map((tag) => (
        <Link key={tag} to={`/search?tag=${encodeURIComponent(tag)}`}>
          <Badge variant="secondary" className="tag-chip">
            {tag}
          </Badge>
        </Link>
      ))}
    </div>
  )
}
export function DateText({ value }: { value: string }) {
  return <time dateTime={value}>{new Date(value).toLocaleString('zh-CN')}</time>
}
export function Collection<T>({
  path,
  children,
  empty = '暂无内容。',
  renderItems,
}: {
  path: string
  children: (item: T) => ReactNode
  renderItems?: (rows: ReactNode[]) => ReactNode
  empty?: ReactNode
}) {
  const [cursors, setCursors] = useState<string[]>([''])
  const [revision, setRevision] = useState(0)
  const cursor = cursors[cursors.length - 1]
  const resource = useResource<{ items: T[]; next_cursor: string | null }>(
    `${path}${path.includes('?') ? '&' : '?'}limit=20${cursor ? '&cursor=' + encodeURIComponent(cursor) : ''}`,
    revision,
  )
  if (!resource.data)
    return (
      <>
        <Loading error={resource.error} />
        {resource.error && (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setCursors([''])
              setRevision((v) => v + 1)
            }}
          >
            重新读取列表
          </Button>
        )}
      </>
    )
  const { items, next_cursor } = resource.data
  return (
    <>
      <div className="collection">
        {items.length ? (
          renderItems ? (
            renderItems(items.map(children))
          ) : (
            items.map(children)
          )
        ) : (
          <p className="empty">{empty}</p>
        )}
      </div>
      {(next_cursor || cursors.length > 1) && (
        <nav className="pagination" aria-label="分页">
          <Button
            type="button"
            variant="outline"
            disabled={cursors.length === 1}
            onClick={() => setCursors((c) => c.slice(0, -1))}
          >
            上一页
          </Button>
          <span>第 {cursors.length} 页</span>
          <Button
            type="button"
            variant="outline"
            disabled={!next_cursor}
            onClick={() => next_cursor && setCursors((c) => [...c, next_cursor])}
          >
            下一页 <ArrowRight size={15} aria-hidden />
          </Button>
        </nav>
      )}
    </>
  )
}
