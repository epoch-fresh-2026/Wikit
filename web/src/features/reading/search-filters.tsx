import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { SelectField } from '@/components/shared'
import { TagInput, appendTag } from '@/features/editing/tag-input'
import { query } from '@/lib/api'

export function SearchFilters({
  params,
  apply,
}: {
  params: URLSearchParams
  apply: (q: string) => void
}) {
  const [q, setQ] = useState(params.get('q') ?? '')
  const [tags, setTags] = useState(
    params.has('tags') ? params.getAll('tags') : params.get('tag') ? [params.get('tag')!] : [],
  )
  const [exclude, setExclude] = useState(params.getAll('exclude_tags'))
  const [match, setMatch] = useState(params.get('tag_match') ?? 'all')
  const [type, setType] = useState(params.get('type') ?? '')
  const [tag, setTag] = useState(''),
    [excluded, setExcluded] = useState('')
  return (
    <form
      className="stack"
      onSubmit={(e) => {
        e.preventDefault()
        const included = appendTag(tags, tag),
          omitted = appendTag(exclude, excluded)
        setTags(included)
        setExclude(omitted)
        setTag('')
        setExcluded('')
        apply(
          query({
            q: q.trim(),
            tags: included,
            tag_match: match,
            exclude_tags: omitted,
            type,
          }),
        )
      }}
    >
      <Input
        aria-label="搜索关键词"
        placeholder="搜索页面"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <TagInput label="包含标签" tags={tags} value={tag} onTags={setTags} onValue={setTag} />
      <SelectField
        label="匹配方式"
        value={match}
        onChange={setMatch}
        options={[
          ['all', '全部满足'],
          ['any', '任一满足'],
        ]}
      />
      <TagInput
        label="排除标签"
        tags={exclude}
        value={excluded}
        onTags={setExclude}
        onValue={setExcluded}
      />
      <small>包含与排除标签合计最多 30 个。排除标签任一命中即剔除。</small>
      <SelectField
        label="页面类型"
        value={type}
        onChange={setType}
        options={[
          ['', '全部类型'],
          ['document', '文档'],
          ['media', '媒体'],
        ]}
      />
      <div className="toolbar">
        <Button>搜索</Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setQ('')
            setTags([])
            setExclude([])
            setMatch('all')
            setType('')
            setTag('')
            setExcluded('')
            apply('')
          }}
        >
          清空筛选
        </Button>
      </div>
    </form>
  )
}
