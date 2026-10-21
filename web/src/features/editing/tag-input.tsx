import { useId, useRef } from 'react'
import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

export function appendTag(tags: string[], value: string) {
  const tag = value.trim().normalize('NFC')
  return tag && !tags.includes(tag) ? [...tags, tag] : tags
}
export function TagInput({
  tags,
  value,
  onValue,
  onTags,
}: {
  tags: string[]
  value: string
  onValue: (v: string) => void
  onTags: (v: string[]) => void
}) {
  const id = useId(),
    composing = useRef(false)
  return (
    <div className="field">
      <Label htmlFor={id}>标签</Label>
      <div className="tag-editor">
        {tags.map((tag, index) => (
          <Badge variant="secondary" key={`${tag}:${index}`}>
            {tag}
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`移除标签 ${tag}`}
              onClick={() => onTags(tags.filter((_, i) => i !== index))}
            >
              <X />
            </Button>
          </Badge>
        ))}
        <Input
          id={id}
          value={value}
          placeholder="输入标签后按回车"
          aria-describedby={`${id}-hint`}
          onChange={(e) => onValue(e.target.value)}
          onCompositionStart={() => {
            composing.current = true
          }}
          onCompositionEnd={() => {
            composing.current = false
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              if (!composing.current && !e.nativeEvent.isComposing && e.keyCode !== 229) {
                onTags(appendTag(tags, value))
                onValue('')
              }
            }
          }}
        />
      </div>
      <small id={`${id}-hint`}>最多 30 个标签，每个最多 64 字符。</small>
    </div>
  )
}
