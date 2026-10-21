import { afterEach, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { Markdown, Media, safeURL, uploadFile } from './markdown'
import { api } from '@/lib/api'
import { json, pageID, revisionID } from '@/test/fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  api.setCSRF('')
  api.onUnauthorized = () => {}
})
it('never runs raw HTML or dangerous Markdown protocols and does not fetch third-party images', () => {
  render(
    <Markdown
      text={
        '# 预览\n<script>alert(1)</script>\n\n[危险](javascript:alert%281%29)\n\n![外部图片](https://tracking.example/image)\n\n[正常链接](https://example.com)'
      }
    />,
  )
  expect(document.querySelector('script')).toBeNull()
  expect(document.querySelector('img')).toBeNull()
  expect(screen.getByText('危险').closest('a')).toBeNull()
  expect(screen.getByRole('link', { name: '正常链接' })).toHaveAttribute(
    'href',
    'https://example.com',
  )
  for (const unsafe of [
    'data:text/html,x',
    'javascript:alert(1)',
    '//evil.example',
    '/\\evil.example',
    'https:\n//evil.example',
  ])
    expect(safeURL(unsafe)).toBe('')
})
it('retains tables and code and renders ordinary text safely', () => {
  render(
    <Markdown
      text={'| 一 | 二 |\n| --- | --- |\n| A | B |\n\n```html\n<img onerror="bad()">\n```'}
    />,
  )
  expect(screen.getByRole('table')).toBeInTheDocument()
  expect(screen.getByText('<img onerror="bad()">')).toBeInTheDocument()
  expect(document.querySelector('img')).toBeNull()
})
it('displays protected inline media without invalid paragraph nesting and releases bytes on unmount', async () => {
  const revoke = vi.fn(),
    OriginalURL = URL
  vi.stubGlobal(
    'URL',
    class extends OriginalURL {
      static createObjectURL() {
        return 'blob:http://localhost/protected-media'
      }
      static revokeObjectURL = revoke
    },
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response('image', { headers: { 'Content-Type': 'image/png' } })),
  )
  const view = render(
    <Markdown text={`![受限原件](/api/v1/pages/${pageID}/revisions/${revisionID}/content)`} />,
  )
  expect(await screen.findByRole('img', { name: '受限原件' })).toHaveAttribute(
    'src',
    'blob:http://localhost/protected-media',
  )
  expect(document.querySelector('p figure')).toBeNull()
  view.unmount()
  expect(revoke).toHaveBeenCalledWith('blob:http://localhost/protected-media')
})
it('validates fixed media URLs, handles revoked access, and cancels an old media read', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 404 }))
  vi.stubGlobal('fetch', fetcher)
  const view = render(<Media url={`/api/v1/pages/${pageID}/revisions/${revisionID}/content`} />)
  await screen.findByText('媒体不可用或已无权访问。')
  const init = fetcher.mock.calls[0][1] as RequestInit
  expect(init.cache).toBe('no-store')
  expect(init.credentials).toBe('same-origin')
  view.rerender(<Media url="https://evil.invalid/content" />)
  expect(init.signal?.aborted).toBe(true)
  expect(fetcher).toHaveBeenCalledOnce()
})
it('uses the same authentication reset for a 401 media response as for JSON responses', async () => {
  const unauthorized = vi.fn()
  api.onUnauthorized = unauthorized
  api.setCSRF('old-nonce')
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })))
  render(<Media url={`/api/v1/pages/${pageID}/revisions/${revisionID}/content`} />)
  await screen.findByText('媒体不可用或已无权访问。')
  expect(unauthorized).toHaveBeenCalledOnce()
  expect(api.csrf).toBe('')
})
it('uploads original bytes through the reserved same-origin HTTP path using CSRF', async () => {
  const path = `/api/v1/uploads/${revisionID}/content`,
    file = new File(['original bytes'], 'a.png', { type: 'image/png' })
  const fetcher = vi
    .fn()
    .mockResolvedValue(
      json({ id: revisionID, size: 14, mime: 'image/png', sha256: 'a'.repeat(64) }),
    )
  vi.stubGlobal('fetch', fetcher)
  api.setCSRF('upload-csrf')
  const reserve = vi.fn().mockResolvedValue({
    id: pageID,
    state: 'pending',
    upload_path: path,
    expires_at: '',
    blob_id: null,
  })
  const out = await uploadFile(file, reserve)
  expect(out.id).toBe(revisionID)
  expect(reserve).toHaveBeenCalledWith({ filename: 'a.png', size: 14, mime: 'image/png' })
  const [url, init] = fetcher.mock.calls[0] as [string, RequestInit]
  expect(url).toBe(path)
  expect(init.method).toBe('PUT')
  expect(init.body).toBe(file)
  expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('upload-csrf')
  expect(() => api.upload('https://evil.invalid/content', file)).toThrow()
  await waitFor(() => expect(fetcher).toHaveBeenCalledOnce())
})
