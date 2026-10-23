import { useState, useRef, useDeferredValue, memo } from 'react'
import { type Schema } from '@/lib/api'
import { useAction } from '@/lib/hooks'
import { ActionFeedback, Field, SelectField } from '@/components/shared'
import { TagInput, appendTag } from './tag-input'
import { LeaveGuard } from './leave-guard'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Markdown, uploadFile } from '@/features/reading/markdown'

const MarkdownPreview = memo(Markdown)

export const emptySnapshot: Schema<'Snapshot'> = {
  type: 'document',
  title: '',
  tags: [],
  markdown: '',
}
export function SnapshotEditor({
  initial = emptySnapshot,
  submit,
  action,
  label = '提交提案',
  allowType = false,
  submitDisabled = false,
}: {
  initial?: Schema<'Snapshot'>
  submit: (snapshot: Schema<'Snapshot'>, published: () => void) => Promise<void>
  action: ReturnType<typeof useAction>
  label?: string
  allowType?: boolean
  submitDisabled?: boolean
}) {
  const [type, setType] = useState(initial.type),
    [title, setTitle] = useState(initial.title),
    [tags, setTags] = useState(initial.tags)
  const [tagInput, setTagInput] = useState(''),
    saving = useRef(false)
  const [text, setText] = useState(
    initial.type === 'document' ? initial.markdown : initial.description,
  )
  const [blob, setBlob] = useState(initial.type === 'media' ? initial.blob_id : '')
  const [view, setView] = useState<'edit' | 'preview'>('edit')
  const previewText = useDeferredValue(text)
  const upload = useAction()
  const dirty =
    type !== initial.type ||
    title !== initial.title ||
    JSON.stringify(tags) !== JSON.stringify(initial.tags) ||
    !!tagInput ||
    text !== (initial.type === 'document' ? initial.markdown : initial.description) ||
    blob !== (initial.type === 'media' ? initial.blob_id : '')
  return (
    <>
      <LeaveGuard dirty={dirty} saving={saving} />
      <form
        className="stack snapshot-editor"
        onSubmit={(e) => {
          e.preventDefault()
          if (upload.busy || submitDisabled) return
          void action.run(async () => {
            const base = {
              title,
              tags: appendTag(tags, tagInput),
            }
            try {
              await submit(
                type === 'document'
                  ? { ...base, type, markdown: text }
                  : { ...base, type, description: text, blob_id: blob },
                () => {
                  saving.current = true
                },
              )
            } finally {
              saving.current = false
            }
          })
        }}
      >
        {allowType && (
          <SelectField
            label="页面类型"
            value={type}
            onChange={(v) => setType(v as typeof type)}
            options={[
              ['document', '文档'],
              ['media', '媒体'],
            ]}
          />
        )}
        <Field label="标题" value={title} onChange={setTitle} required />
        <TagInput tags={tags} value={tagInput} onValue={setTagInput} onTags={setTags} />
        {type === 'media' && (
          <div className="panel stack">
            <label htmlFor="media-file">上传媒体原件</label>
            <Input
              id="media-file"
              type="file"
              accept="image/jpeg,image/png,image/gif,image/webp,audio/mpeg,audio/wav,audio/ogg,video/mp4,video/webm,application/pdf"
              disabled={upload.busy || action.busy}
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (!file) return
                void upload.run(async () => {
                  const out = await uploadFile(file, (body) =>
                    upload.write<Schema<'Upload'>>('uploads', body),
                  )
                  setBlob(out.id)
                  upload.setNotice('原件上传完成。')
                })
              }}
            />
            <p className="muted">支持图片、音视频及 PDF；默认上限 100 MiB，以站点配置为准。</p>
            {blob && <small className="id">原件 ID：{blob}</small>}
            <ActionFeedback action={upload} />
          </div>
        )}
        <div className="editor-view-switch" role="group" aria-label="编辑视图">
          <Button
            type="button"
            variant={view === 'edit' ? 'secondary' : 'ghost'}
            aria-pressed={view === 'edit'}
            onClick={() => setView('edit')}
          >
            编辑正文
          </Button>
          <Button
            type="button"
            variant={view === 'preview' ? 'secondary' : 'ghost'}
            aria-pressed={view === 'preview'}
            onClick={() => setView('preview')}
          >
            查看预览
          </Button>
        </div>
        <div className="editor-grid" data-view={view}>
          <div className="editor-input">
            <Field
              label={type === 'document' ? 'Markdown 正文' : '媒体描述（Markdown）'}
              value={text}
              onChange={setText}
              multiline
            />
            <p className="muted">支持 Markdown；媒体引用请使用固定版本链接。</p>
          </div>
          <section className="preview" aria-label="Markdown 预览" aria-busy={text !== previewText}>
            <h2>预览</h2>
            {previewText ? (
              <MarkdownPreview text={previewText} />
            ) : (
              <p className="preview-placeholder">写下内容后，这里会显示阅读效果。</p>
            )}
          </section>
        </div>
        <div className="editor-submit">
          <span className="muted">{dirty ? '有未提交的修改' : '尚未修改内容'}</span>
          <Button
            disabled={submitDisabled || action.busy || upload.busy || (type === 'media' && !blob)}
          >
            {action.busy ? '正在提交…' : label}
          </Button>
        </div>
        <ActionFeedback action={action} />
      </form>
    </>
  )
}
