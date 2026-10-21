import MarkdownRenderer from 'react-markdown'
import { Link } from 'react-router'
import remarkGfm from 'remark-gfm'
import { useEffect, useState } from 'react'
import { api, type Schema } from '@/lib/api'

const fixedContent =
  /^\/api\/v1\/(?:pages\/[1-9]\d*\/revisions\/[a-f\d-]{36}|proposals\/[a-f\d-]{36}\/revisions\/[a-f\d-]{36})\/content$/i
export function safeURL(value: string): string {
  if (!value || /[\u0000-\u0020\\]/.test(value) || value.startsWith('//')) return ''
  if (value.startsWith('/') || value.startsWith('#')) return value
  try {
    return ['https:', 'http:', 'mailto:'].includes(new URL(value).protocol) ? value : ''
  } catch {
    return ''
  }
}
// Fetch media with no-store rather than exposing an authenticated URL to caches.
export function Media({
  url,
  alt = '媒体原件',
  mime,
}: {
  url: string
  alt?: string
  mime?: string
}) {
  const [objectURL, setObjectURL] = useState(''),
    [type, setType] = useState(mime ?? ''),
    [failed, setFailed] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    let active = true,
      localURL = ''
    setFailed(false)
    setObjectURL('')
    if (!fixedContent.test(url)) {
      setFailed(true)
      return
    }
    // The endpoint independently rechecks current page permissions.
    api
      .media(url, controller.signal)
      .then((blob) => {
        if (!active) return
        localURL = URL.createObjectURL(blob)
        setType(blob.type)
        setObjectURL(localURL)
      })
      .catch(() => {
        if (active) setFailed(true)
      })
    return () => {
      active = false
      controller.abort()
      if (localURL) URL.revokeObjectURL(localURL)
    }
  }, [url])
  if (failed) return <span role="status">媒体不可用或已无权访问。</span>
  if (!objectURL) return <span role="status">正在读取媒体…</span>
  return (
    <span className="media">
      {type.startsWith('image/') ? (
        <img src={objectURL} alt={alt} />
      ) : type.startsWith('audio/') ? (
        <audio src={objectURL} controls aria-label={alt} />
      ) : type.startsWith('video/') ? (
        <video src={objectURL} controls aria-label={alt} />
      ) : (
        <span>PDF 原件可通过下方链接打开。</span>
      )}
      <span className="media-caption">
        <a href={objectURL} download={alt}>
          下载原件
        </a>
      </span>
    </span>
  )
}
export function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <MarkdownRenderer
        skipHtml
        remarkPlugins={[remarkGfm]}
        urlTransform={safeURL}
        components={{
          a: ({ href, children }) =>
            href?.startsWith('/') && !href.startsWith('/api/') ? (
              <Link to={href}>{children}</Link>
            ) : href ? (
              <a href={href} rel="noreferrer">
                {children}
              </a>
            ) : (
              <span>{children}</span>
            ),
          img: ({ src, alt }) =>
            typeof src === 'string' && fixedContent.test(src) ? (
              <Media url={src} alt={alt} />
            ) : (
              <span>{alt || '外部图片'}（请使用固定媒体版本）</span>
            ),
        }}
      >
        {text}
      </MarkdownRenderer>
    </div>
  )
}
export function SnapshotView({
  snapshot,
  contentURL,
}: {
  snapshot: Schema<'Snapshot'>
  contentURL?: string | null
}) {
  return (
    <>
      {snapshot.type === 'document' ? (
        <Markdown text={snapshot.markdown} />
      ) : (
        <>
          {contentURL && <Media url={contentURL} alt={snapshot.title} />}
          <Markdown text={snapshot.description} />
        </>
      )}
    </>
  )
}
// Reservation and upload are explicit, and share the snapshot editor's retry key.
export async function uploadFile(
  file: File,
  reserve: (body: Schema<'UploadInput'>) => Promise<Schema<'Upload'>>,
) {
  const upload = await reserve({
    filename: file.name,
    size: file.size,
    mime: file.type as Schema<'UploadInput'>['mime'],
  })
  return await api.upload(upload.upload_path, file)
}
