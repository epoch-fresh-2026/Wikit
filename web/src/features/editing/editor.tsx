import { useState } from 'react'
import { type Schema } from '@/lib/api'
import { useAction } from '@/lib/hooks'
import { ActionFeedback, Field, SelectField } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Markdown, uploadFile } from '@/features/reading/markdown'

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
}: {
  initial?: Schema<'Snapshot'>
  submit: (snapshot: Schema<'Snapshot'>) => Promise<void>
  action: ReturnType<typeof useAction>
  label?: string
  allowType?: boolean
}) {
  const [type, setType] = useState(initial.type),
    [title, setTitle] = useState(initial.title),
    [tags, setTags] = useState(initial.tags.join(', '))
  const [text, setText] = useState(
    initial.type === 'document' ? initial.markdown : initial.description,
  )
  const [blob, setBlob] = useState(initial.type === 'media' ? initial.blob_id : '')
  const upload = useAction()
  return (
    <form
      className="stack"
      onSubmit={(e) => {
        e.preventDefault()
        if (upload.busy) return
        void action.run(async () => {
          const base = {
            title,
            tags: tags
              .split(/[,，]/)
              .map((t) => t.trim())
              .filter(Boolean),
          }
          await submit(
            type === 'document'
              ? { ...base, type, markdown: text }
              : { ...base, type, description: text, blob_id: blob },
          )
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
      <Field label="标签" value={tags} onChange={setTags} hint="用逗号分隔，例如：Go, 知识管理" />
      {type === 'media' && (
        <div className="panel stack">
          <label htmlFor="media-file">上传媒体原件</label>
          <input
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
                upload.setNotice('原件上传完成。提交提案后仍需审核才能发布。')
              })
            }}
          />
          <p className="muted">支持图片、音视频及 PDF；默认上限 100 MiB，以站点配置为准。</p>
          {blob && <small className="id">原件 ID：{blob}</small>}
          <ActionFeedback action={upload} />
        </div>
      )}
      <div className="editor-grid">
        <div>
          <Field
            label={type === 'document' ? 'Markdown 正文' : '媒体描述（Markdown）'}
            value={text}
            onChange={setText}
            multiline
          />
          <p className="muted">支持 Markdown。原始 HTML 不执行；内部媒体请使用固定发布版本链接。</p>
        </div>
        <section className="preview" aria-label="Markdown 预览">
          <h2>预览</h2>
          <Markdown text={text} />
        </section>
      </div>
      <Button disabled={action.busy || upload.busy || (type === 'media' && !blob)}>{label}</Button>
      <ActionFeedback action={action} />
    </form>
  )
}
