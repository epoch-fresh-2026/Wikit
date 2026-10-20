import { afterEach, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Collection } from './shared'
import { failure, json } from '@/test/fixtures'
afterEach(() => vi.unstubAllGlobals())
it('uses opaque cursors, clears prior content on a rejected cursor, and restarts on explicit retry', async () => {
  const fetcher = vi.fn(async (path: string) =>
    path.includes('cursor=')
      ? failure('invalid_request', 400)
      : json({ items: [{ title: 'first page title' }], next_cursor: 'opaque+/cursor' }),
  )
  vi.stubGlobal('fetch', fetcher)
  render(
    <Collection<{ title: string }> path="pages?tag=Go">
      {(item) => <p key={item.title}>{item.title}</p>}
    </Collection>,
  )
  const user = userEvent.setup()
  await screen.findByText('first page title')
  await user.click(screen.getByRole('button', { name: '下一页' }))
  await screen.findByRole('alert')
  expect(screen.queryByText('first page title')).not.toBeInTheDocument()
  expect(fetcher.mock.calls[1][0]).toContain('cursor=opaque%2B%2Fcursor')
  await user.click(screen.getByRole('button', { name: '重新读取列表' }))
  await screen.findByText('first page title')
  expect(fetcher.mock.calls[2][0]).not.toContain('cursor=')
})
