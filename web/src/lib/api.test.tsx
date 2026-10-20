import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { api, ApiError } from './api'
import { useAction, useResource } from './hooks'
import { json, failure } from '@/test/fixtures'

beforeEach(() => {
  api.setCSRF('test-csrf')
  api.onUnauthorized = () => {}
})
afterEach(() => {
  vi.unstubAllGlobals()
  api.setCSRF('')
})
describe('same-origin client', () => {
  it('sends Cookie/CSRF and idempotency headers without Bearer or persistent secrets', async () => {
    const fetcher = vi.fn().mockResolvedValue(json({ ok: true }))
    vi.stubGlobal('fetch', fetcher)
    await api.send('pages', { title: 'draft' }, 'POST', 'retry-key')
    const [url, init] = fetcher.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/pages')
    expect(init.credentials).toBe('same-origin')
    expect(init.cache).toBe('no-store')
    expect(init.redirect).toBe('error')
    const headers = new Headers(init.headers)
    expect(headers.get('X-CSRF-Token')).toBe('test-csrf')
    expect(headers.get('Idempotency-Key')).toBe('retry-key')
    expect(headers.has('Authorization')).toBe(false)
    expect(Object.keys(sessionStorage)).toEqual(['wikit.csrf'])
    await expect(api.request('https://example.com/api/v1/pages')).rejects.toMatchObject({
      code: 'invalid_request',
    })
  })
  it('refuses writes without CSRF and clears authentication on 401', async () => {
    const unauthorized = vi.fn()
    api.onUnauthorized = unauthorized
    const fetcher = vi.fn().mockResolvedValue(failure('unauthenticated', 401))
    vi.stubGlobal('fetch', fetcher)
    api.setCSRF('')
    await expect(api.send('pages', {})).rejects.toMatchObject({ code: 'csrf_missing' })
    expect(fetcher).not.toHaveBeenCalled()
    api.setCSRF('old')
    await expect(api.get('auth/me')).rejects.toMatchObject({
      code: 'unauthenticated',
      requestID: 'test-request-id',
    })
    expect(unauthorized).toHaveBeenCalledOnce()
    expect(api.csrf).toBe('')
  })
  it('only presents stable errors, never raw SQL, request bodies or token diagnostics', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(failure('internal_error', 500)))
    await expect(api.get('pages')).rejects.toMatchObject({
      message: '服务暂时不可用，请保留草稿后重试。',
    })
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('secret-token-sql')))
    await expect(api.get('pages')).rejects.toBeInstanceOf(ApiError)
    await expect(api.get('pages')).rejects.not.toThrow('secret-token-sql')
  })
})
it('keeps a failed write key, changes it for changed content, and blocks duplicate submissions', async () => {
  const fetcher = vi.fn().mockRejectedValue(new Error('offline'))
  vi.stubGlobal('fetch', fetcher)
  function Writer() {
    const action = useAction()
    return (
      <>
        <button
          onClick={() =>
            void action.run(async () => {
              await action.write('pages', { title: 'a' })
            })
          }
        >
          重试
        </button>
        <button
          onClick={() =>
            void action.run(async () => {
              await action.write('pages', { title: 'b' })
            })
          }
        >
          修改
        </button>
        {action.error && <p role="alert">{action.error.message}</p>}
      </>
    )
  }
  render(<Writer />)
  const user = userEvent.setup()
  await user.click(screen.getByText('重试'))
  await screen.findByRole('alert')
  await user.click(screen.getByText('重试'))
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2))
  const key = (n: number) =>
    new Headers((fetcher.mock.calls[n][1] as RequestInit).headers).get('Idempotency-Key')
  expect(key(0)).toBe(key(1))
  await user.click(screen.getByText('修改'))
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3))
  expect(key(2)).not.toBe(key(0))
  let resolvePending: (r: Response) => void = () => {}
  fetcher.mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        resolvePending = resolve
      }),
  )
  await user.dblClick(screen.getByText('重试'))
  expect(fetcher).toHaveBeenCalledTimes(4)
  await act(async () => {
    resolvePending(json({ ok: true }))
  })
})
it('cancels old reads and ignores a late response after changing objects', async () => {
  let resolveOld: (r: Response) => void = () => {}
  const fetcher = vi.fn().mockImplementation((path: string) =>
    path.endsWith('old')
      ? new Promise<Response>((resolve) => {
          resolveOld = resolve
        })
      : Promise.resolve(json({ title: 'new' })),
  )
  vi.stubGlobal('fetch', fetcher)
  function Reader({ path }: { path: string }) {
    const state = useResource<{ title: string }>(path)
    return <p>{state.data?.title ?? 'loading'}</p>
  }
  const view = render(<Reader path="pages/old" />)
  await waitFor(() => expect(fetcher).toHaveBeenCalledOnce())
  const oldSignal = (fetcher.mock.calls[0][1] as RequestInit).signal
  view.rerender(<Reader path="pages/new" />)
  expect(oldSignal?.aborted).toBe(true)
  await screen.findByText('new')
  resolveOld(json({ title: 'secret old content' }))
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(screen.queryByText('secret old content')).not.toBeInTheDocument()
})
