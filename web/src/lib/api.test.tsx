import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { api, ApiError } from './api'
import { useAction, useResource } from './hooks'
import { account, json, failure } from '@/test/fixtures'

beforeEach(() => {
  api.setCSRF('test-csrf')
  api.onUnauthorized = () => {}
  api.onSessionRestored = () => true
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
  it('clears authentication only after session recovery confirms a 401', async () => {
    const unauthorized = vi.fn()
    api.onUnauthorized = unauthorized
    const fetcher = vi
      .fn()
      .mockImplementation(() => Promise.resolve(failure('unauthenticated', 401)))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.get('auth/me')).rejects.toMatchObject({ code: 'unauthenticated' })
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(unauthorized).toHaveBeenCalledOnce()
    expect(api.csrf).toBe('')
  })
  it('coalesces missing-CSRF recovery before concurrent writes and preserves their keys', async () => {
    api.setCSRF('')
    const fetcher = vi.fn(async (path: string) =>
      path.endsWith('/auth/session')
        ? json({ account, csrf_token: 'restored' })
        : json({ ok: true }),
    )
    vi.stubGlobal('fetch', fetcher)
    await Promise.all([api.send('pages', {}, 'POST', 'one'), api.send('pages', {}, 'POST', 'two')])
    expect(fetcher.mock.calls.filter(([path]) => path.endsWith('/auth/session'))).toHaveLength(1)
    expect(api.csrf).toBe('restored')
    for (const [, init] of vi.mocked(fetch).mock.calls.slice(1)) {
      expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('restored')
      expect(new Headers(init?.headers).get('Idempotency-Key')).toMatch(/one|two/)
    }
  })
  it('does not clear identity on recovery transport failure and allows a later retry', async () => {
    api.setCSRF('')
    const unauthorized = vi.fn()
    api.onUnauthorized = unauthorized
    const fetcher = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(json({ account, csrf_token: 'restored' }))
      .mockResolvedValue(json({ ok: true }))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.send('pages', {})).rejects.toMatchObject({ code: 'network_error' })
    expect(unauthorized).not.toHaveBeenCalled()
    await api.send('pages', {})
    expect(fetcher).toHaveBeenCalledTimes(3)
  })
  it.each(['', 'new-login'])(
    'discards late recovery after credentials change to %s',
    async (newCSRF) => {
      api.setCSRF('')
      let finish!: (value: Response) => void
      const fetcher = vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve
          }),
      )
      vi.stubGlobal('fetch', fetcher)
      const write = api.send('pages', {})
      api.setCSRF(newCSRF)
      finish(json({ account, csrf_token: 'old-session' }))
      await expect(write).rejects.toMatchObject({ code: 'session_changed' })
      expect(api.csrf).toBe(newCSRF)
      expect(fetcher).toHaveBeenCalledOnce()
    },
  )
  it('blocks an old-account write when recovery discovers another identity', async () => {
    api.setCSRF('')
    api.onSessionRestored = () => false
    const fetcher = vi.fn().mockResolvedValue(json({ account, csrf_token: 'new-account' }))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.send('pages', {})).rejects.toMatchObject({ code: 'session_changed' })
    expect(fetcher).toHaveBeenCalledOnce()
  })
  it('restores a stale nonce after 403 without replaying the failed mutation', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(failure('forbidden', 403))
      .mockResolvedValueOnce(json({ account, csrf_token: 'restored' }))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.send('pages', {}, 'POST', 'retry-key')).rejects.toMatchObject({
      code: 'write_credentials_restored',
    })
    expect(api.csrf).toBe('restored')
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
  it('keeps identity when a 401 cannot be confirmed because recovery is offline', async () => {
    const unauthorized = vi.fn()
    api.onUnauthorized = unauthorized
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(failure('unauthenticated', 401))
      .mockRejectedValueOnce(new Error('offline'))
    vi.stubGlobal('fetch', fetcher)
    await expect(api.send('pages', {})).rejects.toMatchObject({ code: 'network_error' })
    expect(unauthorized).not.toHaveBeenCalled()
    expect(api.csrf).toBe('test-csrf')
  })
  it('ignores a late invalid-session response after a new login', async () => {
    api.setCSRF('')
    let finish!: (value: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve
          }),
      ),
    )
    const unauthorized = vi.fn()
    api.onUnauthorized = unauthorized
    const write = api.send('pages', {})
    api.setCSRF('new-login')
    finish(failure('unauthenticated', 401))
    await expect(write).rejects.toMatchObject({ code: 'session_changed' })
    expect(api.csrf).toBe('new-login')
    expect(unauthorized).not.toHaveBeenCalled()
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
