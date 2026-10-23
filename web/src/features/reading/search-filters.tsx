import { useId, useState } from 'react'
import { ChevronDown } from 'lucide-react'
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
  const panelID = useId()
  const [expanded, setExpanded] = useState(false)
  const includedCount = params.has('tags')
    ? params.getAll('tags').length
    : params.get('tag')
      ? 1
      : 0
  const excludedCount = params.getAll('exclude_tags').length
  const summary = [
    includedCount
      ? `包含 ${includedCount} 个标签（${params.get('tag_match') === 'any' ? '任一满足' : '全部满足'}）`
      : '',
    excludedCount ? `排除 ${excludedCount} 个标签` : '',
    params.get('type') === 'document' ? '文档' : params.get('type') === 'media' ? '媒体' : '',
  ]
    .filter(Boolean)
    .join(' · ')
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
      className="search-filters"
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
      <div className="search-query">
        <Input
          aria-label="搜索关键词"
          placeholder="搜索页面"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <Button>搜索</Button>
      </div>
      <div className="search-filter-toolbar">
        <Button
          type="button"
          variant="outline"
          aria-expanded={expanded}
          aria-controls={panelID}
          onClick={() => setExpanded(!expanded)}
        >
          筛选
          <ChevronDown aria-hidden="true" className={expanded ? 'rotate-180' : ''} />
        </Button>
        {summary && <small className="search-filter-summary">已应用：{summary}</small>}
        <Button
          type="button"
          variant="ghost"
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
      <div id={panelID} hidden={!expanded}>
        <div className="search-filter-panel">
          <TagInput label="包含标签" tags={tags} value={tag} onTags={setTags} onValue={setTag} />
          <TagInput
            label="排除标签"
            tags={exclude}
            value={excluded}
            onTags={setExclude}
            onValue={setExcluded}
          />
          <SelectField
            label="匹配方式"
            value={match}
            onChange={setMatch}
            options={[
              ['all', '全部满足'],
              ['any', '任一满足'],
            ]}
          />
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
          <small className="search-filter-hint">
            包含与排除标签合计最多 30 个。排除标签任一命中即剔除。
          </small>
        </div>
      </div>
    </form>
  )
}
