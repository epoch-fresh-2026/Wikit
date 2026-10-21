import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { App } from '@/app'
import { AuthProvider } from '@/features/account/auth'
import { api, type Schema } from '@/lib/api'
import {
  account,
  candidateID,
  detail,
  failure,
  json,
  list,
  pageID,
  proposal,
  proposalID,
  revision,
  revisionID,
} from './fixtures'

type Handler = (path: string, init: RequestInit) => Response | Promise<Response> | undefined
function mount(route = '/', handler?: Handler, anonymous = false) {
  const fetcher = vi.fn(async (input: string, init: RequestInit) => {
    const path = String(input),
      overridden = handler?.(path, init)
    if (overridden) return overridden
    if (path.endsWith('/auth/me'))
      return anonymous ? failure('unauthenticated', 401) : json(account)
    if (path.includes('/auth/token'))
      return json({ active: true, generation: 1, created_at: revision.created_at })
    if (path === '/api/v1/pages/1' || path === `/api/v1/pages/${pageID}`) return json(detail)
    if (path === `/api/v1/proposals/${proposalID}`) return json(proposal)
    if (path.endsWith('/diff'))
      return json({
        base_revision_id: revisionID,
        head_revision_id: revisionID,
        proposal_revision_id: candidateID,
        stale: false,
        fields: [],
      })
    if (path === `/api/v1/pages/${pageID}/revisions/${revisionID}`) return json(revision)
    if (path.startsWith(`/api/v1/pages/${pageID}/revisions?`)) return json(list([revision]))
    if (path.startsWith(`/api/v1/proposals/${proposalID}/revisions?`))
      return json(list([proposal.current_revision]))
    if (
      path.startsWith('/api/v1/proposals?') ||
      path.startsWith(`/api/v1/pages/${pageID}/proposals?`)
    )
      return json(list([proposal]))
    if (path.startsWith('/api/v1/tags?')) return json(list([{ key: '知识' }]))
    if (path.startsWith('/api/v1/pages?')) return json(list([detail]))
    return failure('not_found', 404)
  })
  vi.stubGlobal('fetch', fetcher)
  render(
    <RouterProvider
      router={createMemoryRouter(
        [
          {
            path: '*',
            element: (
              <AuthProvider>
                <App />
              </AuthProvider>
            ),
          },
        ],
        { initialEntries: [route] },
      )}
    />,
  )
  return { fetcher, user: userEvent.setup() }
}
beforeEach(() => {
  sessionStorage.clear()
  api.setCSRF('browser-csrf')
})
afterEach(() => {
  vi.unstubAllGlobals()
  api.onUnauthorized = () => {}
  api.setCSRF('')
})

it('browses pages and tags and submits a keyboard search without rendering snippets as HTML', async () => {
  const { user, fetcher } = mount('/', (path) =>
    path.startsWith('/api/v1/search?')
      ? json(
          list([
            {
              page_id: pageID,
              revision_id: revisionID,
              title: '搜索结果',
              type: 'document',
              tags: [],
              snippet: '<script>private</script>',
            },
          ]),
        )
      : undefined,
  )
  await screen.findByRole('heading', { name: '协作知识' })
  const search = screen.getByRole('textbox', { name: '搜索页面' })
  await user.click(search)
  await user.type(search, '中文 Go')
  await user.keyboard('{Enter}')
  await screen.findByRole('heading', { name: '搜索结果', level: 1 })
  expect(fetcher.mock.calls.some(([url]) => String(url).includes('q=%E4%B8%AD%E6%96%87+Go'))).toBe(
    true,
  )
  expect(document.querySelector('script')).toBeNull()
  expect(screen.getByText('<script>private</script>')).toBeInTheDocument()
})
it('logs in with Cookie/CSRF and does not persist a password or Bearer token', async () => {
  const { user, fetcher } = mount(
    '/login',
    (path, init) =>
      path.endsWith('/auth/login') && init.method === 'POST'
        ? json({ account, csrf_token: 'fresh-csrf' })
        : undefined,
    true,
  )
  await screen.findByRole('heading', { name: '登录' })
  await user.type(screen.getByLabelText('用户名'), 'alice')
  await user.type(screen.getByLabelText('密码'), 'never-persist-password')
  await user.click(within(screen.getByRole('main')).getByRole('button', { name: '登录' }))
  await screen.findByRole('heading', { name: '协作知识' })
  expect(api.csrf).toBe('fresh-csrf')
  expect(Object.keys(sessionStorage)).toEqual(['wikit.csrf'])
  expect(localStorage.length).toBe(0)
  const call = fetcher.mock.calls.find(([url]) => String(url).endsWith('/auth/login'))!
  expect(JSON.parse(call[1]!.body as string).password).toBe('never-persist-password')
})
it('shows a registration Token only once and removes it on navigation', async () => {
  const { user } = mount(
    '/register',
    (path, init) =>
      path.endsWith('/auth/register') && init.method === 'POST'
        ? json({ account, token: 'one-time-registration-token' }, 201)
        : undefined,
    true,
  )
  await screen.findByRole('heading', { name: '注册' })
  await user.type(screen.getByLabelText('用户名'), 'alice')
  await user.type(screen.getByLabelText('密码'), 'test-password-2026')
  await user.click(screen.getByRole('button', { name: '创建账户' }))
  expect(await screen.findByDisplayValue('one-time-registration-token')).toBeInTheDocument()
  expect(JSON.stringify(sessionStorage)).not.toContain('one-time-registration-token')
  await user.click(screen.getByRole('button', { name: '已保存，前往登录' }))
  await screen.findByRole('heading', { name: '登录' })
  expect(screen.queryByDisplayValue('one-time-registration-token')).not.toBeInTheDocument()
})
it('creates a Markdown page with a preview, then navigates to its proposal', async () => {
  const { user, fetcher } = mount('/pages/new', (path, init) =>
    path === '/api/v1/pages' && init.method === 'POST'
      ? json({ page: detail.page, revision }, 201)
      : undefined,
  )
  await screen.findByRole('heading', { name: '创建页面' })
  await user.type(screen.getByLabelText('标题'), '新知识')
  await user.type(screen.getByLabelText('标签'), 'Go{Enter}知识{Enter}')
  await user.type(screen.getByLabelText('Markdown 正文'), '# 预览标题\n新的正文')
  expect(
    within(screen.getByRole('region', { name: 'Markdown 预览' })).getByRole('heading', {
      name: '预览标题',
    }),
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '创建并发布' }))
  await screen.findByRole('heading', { name: '协作知识' })
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path === '/api/v1/pages' && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string)).toEqual({
    visibility: 'public',
    snapshot: {
      type: 'document',
      title: '新知识',
      tags: ['Go', '知识'],
      markdown: '# 预览标题\n新的正文',
    },
  })
  expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('browser-csrf')
})
it('keeps candidate drafts on stale writes and shows real conflict fields for rebase', async () => {
  const { user, fetcher } = mount(`/pages/${pageID}/proposals/${proposalID}/edit`, (path, init) =>
    init.method === 'POST' && path.endsWith('/revisions')
      ? failure('stale_revision', 409)
      : init.method === 'POST' && path.endsWith('/rebase')
        ? failure('merge_conflict', 409, {
            conflicts: [
              { path: 'markdown', base: 'base line', ours: 'ours line', theirs: 'theirs line' },
            ],
          })
        : undefined,
  )
  const editor = await screen.findByLabelText('Markdown 正文')
  await user.clear(editor)
  await user.type(editor, '保留我的未提交内容')
  await user.click(screen.getByRole('button', { name: '保存修订' }))
  await screen.findByText(/版本已变化/)
  expect(editor).toHaveValue('保留我的未提交内容')
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path.endsWith('/revisions') && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string)).toMatchObject({
    expected_head_revision_id: revisionID,
    expected_proposal_revision_id: candidateID,
  })
  expect(editor).toHaveValue('保留我的未提交内容')
  expect(screen.queryByText('private server diagnostics must not be shown')).not.toBeInTheDocument()
})
it('merges directly with the displayed candidate and head and no review request', async () => {
  const { user, fetcher } = mount(`/pages/${pageID}/proposals/${proposalID}`, (path, init) =>
    init.method === 'POST' && path.endsWith('/merge') ? json(revision) : undefined,
  )
  await waitFor(() => expect(screen.getByRole('button', { name: '合并' })).toBeEnabled())
  expect(screen.queryByRole('button', { name: '提交审核' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '合并' }))
  await screen.findByRole('heading', { name: '协作知识' })
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path.endsWith('/merge') && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string)).toEqual({
    expected_head_revision_id: revisionID,
    expected_proposal_revision_id: candidateID,
  })
  expect(fetcher.mock.calls.some(([path]) => path.includes('/reviews'))).toBe(false)
})
it('clears content and editor subtrees immediately when a request loses authentication', async () => {
  const { user } = mount(`/pages/${pageID}/proposals/${proposalID}/edit`, (path, init) =>
    init.method === 'POST' && path.endsWith('/revisions')
      ? failure('unauthenticated', 401)
      : undefined,
  )
  const editor = await screen.findByLabelText('Markdown 正文')
  await user.clear(editor)
  await user.type(editor, 'confidential draft')
  await user.click(screen.getByRole('button', { name: '保存修订' }))
  await waitFor(() => expect(editor).not.toBeInTheDocument())
  expect(screen.queryByDisplayValue('confidential draft')).not.toBeInTheDocument()
  expect(api.csrf).toBe('')
})
it('allows structured policy editing with current policy version and no client authorization algorithm', async () => {
  const policy: Schema<'Policy'> = {
    visibility: 'restricted',
    policy_version: 7,
    grants: [{ subject_type: 'user', subject_id: account.id, capabilities: ['read'] }],
  }
  const { user, fetcher } = mount(`/pages/${pageID}/policy`, (path, init) =>
    path.endsWith('/policy')
      ? init.method === 'PUT'
        ? failure('forbidden', 403)
        : json(policy)
      : undefined,
  )
  await screen.findByRole('heading', { name: '访问策略' })
  await user.click(screen.getByRole('checkbox', { name: '合并' }))
  await user.click(screen.getByRole('button', { name: '保存页面权限' }))
  await screen.findByText('当前账户没有执行此操作的权限。')
  const [, init] = fetcher.mock.calls.find(([, init]) => init.method === 'PUT')!
  expect(JSON.parse(init.body as string)).toMatchObject({
    expected_policy_version: 7,
    grants: [{ subject_type: 'user', subject_id: account.id, capabilities: ['read', 'merge'] }],
  })
  expect(screen.getByRole('checkbox', { name: '合并' })).toBeChecked()
})
it('renders immutable history and offers restore using the current head', async () => {
  const { user, fetcher } = mount(`/pages/${pageID}/revisions/${revisionID}`, (path, init) =>
    path.endsWith('/restore') && init.method === 'POST' ? json(proposal, 201) : undefined,
  )
  await screen.findByText(/固定发布版本/)
  await waitFor(() => expect(screen.getByRole('button', { name: '创建回滚提案' })).toBeEnabled())
  await user.click(screen.getByRole('button', { name: '创建回滚提案' }))
  await screen.findByRole('heading', { name: '协作知识' })
  const [, init] = fetcher.mock.calls.find(([path]) => path.endsWith('/restore'))!
  expect(JSON.parse(init.body as string)).toEqual({
    revision_id: revisionID,
    expected_head_revision_id: revisionID,
  })
})

it('does not loop on repeated 401 responses after losing an authenticated identity', async () => {
  let revoked = false
  const { user, fetcher } = mount(`/pages/${pageID}/proposals/${proposalID}/edit`, (path, init) => {
    if (init.method === 'POST' && path.endsWith('/revisions')) {
      revoked = true
      return failure('unauthenticated', 401)
    }
    if (revoked) return failure('unauthenticated', 401)
  })
  await screen.findByLabelText('Markdown 正文')
  await user.click(screen.getByRole('button', { name: '保存修订' }))
  await screen.findByRole('alert')
  const count = fetcher.mock.calls.length
  await new Promise((resolve) => setTimeout(resolve, 20))
  expect(fetcher.mock.calls.length).toBe(count)
  expect(count).toBeLessThan(15)
})
it('supports a keyboard-only login form and presents an understandable error', async () => {
  const { user } = mount(
    '/login',
    (path, init) =>
      path.endsWith('/auth/login') && init.method === 'POST'
        ? failure('unauthenticated', 401)
        : undefined,
    true,
  )
  await screen.findByRole('heading', { name: '登录' })
  await waitFor(() => expect(screen.getByRole('main')).toHaveFocus())
  await user.tab()
  expect(screen.getByLabelText('用户名')).toHaveFocus()
  await user.keyboard('alice')
  await user.tab()
  expect(screen.getByLabelText('密码')).toHaveFocus()
  await user.keyboard('wrong-password{Enter}')
  await screen.findByRole('alert')
  expect(screen.queryByText('private server diagnostics must not be shown')).not.toBeInTheDocument()
})
it('offers reauthentication when a fresh tab has a valid Cookie but no CSRF nonce', async () => {
  api.setCSRF('')
  const { user } = mount('/account')
  await screen.findByRole('heading', { name: '个人 Token' })
  await user.click(screen.getByRole('link', { name: '重新登录' }))
  await screen.findByRole('heading', { name: '登录' })
})
it('rotates and clears a one-time Token without putting it in storage', async () => {
  const { user, fetcher } = mount('/account', (path, init) =>
    path.endsWith('/auth/token') && init.method === 'POST'
      ? json({ token: 'fresh-personal-token', generation: 2, created_at: revision.created_at })
      : undefined,
  )
  await screen.findByRole('heading', { name: '个人 Token' })
  await user.type(screen.getByLabelText('当前密码'), 'password-2026')
  await user.click(screen.getByRole('button', { name: '轮换 Token' }))
  await screen.findByDisplayValue('fresh-personal-token')
  expect(JSON.stringify(sessionStorage)).not.toContain('fresh-personal-token')
  expect(localStorage.length).toBe(0)
  await user.click(screen.getByRole('button', { name: '已保存，清除明文' }))
  expect(screen.queryByDisplayValue('fresh-personal-token')).not.toBeInTheDocument()
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path.endsWith('/auth/token') && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string)).toEqual({ current_password: 'password-2026' })
  expect(new Headers(init.headers).has('Idempotency-Key')).toBe(false)
})
it('edits account role and group members using server-provided version conditions', async () => {
  const group: Schema<'Group'> = {
    id: pageID,
    name: '维护组',
    version: 8,
    member_ids: [account.id],
    create_page: false,
  }
  const { user, fetcher } = mount('/admin', (path, init) => {
    if (path.startsWith('/api/v1/admin/accounts?'))
      return json(list([{ account, auth_version: 12 }]))
    if (init.method === 'PATCH') return failure('stale_revision', 409)
    if (path.startsWith('/api/v1/admin/groups?')) return json(list([group]))
    if (init.method === 'PUT') return failure('forbidden', 403)
  })
  await screen.findByRole('heading', { name: 'alice' })
  await user.click(screen.getByLabelText('alice 角色'))
  await user.click(screen.getByRole('option', { name: '版主' }))
  await user.click(screen.getByRole('button', { name: '保存账户变更' }))
  await screen.findByText(/版本已变化/)
  const [, changed] = fetcher.mock.calls.find(([, init]) => init.method === 'PATCH')!
  expect(JSON.parse(changed.body as string)).toEqual({
    role: 'moderator',
    create_page: false,
    status: 'active',
    expected_auth_version: 12,
  })
  await user.click(screen.getByRole('link', { name: '用户组' }))
  await screen.findByRole('heading', { name: '维护组' })
  await user.click(screen.getByRole('checkbox', { name: /alice/ }))
  await user.click(screen.getByRole('button', { name: '保存用户组' }))
  await screen.findByText('当前账户没有执行此操作的权限。')
  const [, grouped] = fetcher.mock.calls.find(([, init]) => init.method === 'PUT')!
  expect(JSON.parse(grouped.body as string)).toEqual({
    name: '维护组',
    member_ids: [],
    create_page: false,
    expected_version: 8,
  })
})
it('creates a page issue, comments without editing old text, and links with an expected version', async () => {
  const issue: Schema<'Issue'> = {
    id: proposalID,
    page_id: pageID,
    author_id: account.id,
    title: '讨论知识',
    body: '议题内容',
    state: 'open',
    hidden: false,
    version: 4,
    proposal_ids: [],
    created_at: revision.created_at,
  }
  const { user, fetcher } = mount(`/pages/${pageID}/issues`, (path, init) => {
    if (path.startsWith(`/api/v1/pages/${pageID}/issues?`)) return json(list([]))
    if (path === `/api/v1/pages/${pageID}/issues` && init.method === 'POST') return json(issue, 201)
    if (path.startsWith(`/api/v1/issues/${proposalID}?`)) return json(issue)
    if (path.includes('/comments?')) return json(list([]))
    if (path.endsWith('/comments') && init.method === 'POST') return failure('forbidden', 403)
    if (path.endsWith('/proposals') && init.method === 'POST') return failure('stale_revision', 409)
  })
  await screen.findByRole('heading', { name: '发起议题' })
  await user.type(screen.getByLabelText('议题标题'), '讨论知识')
  await user.type(screen.getByLabelText('议题内容'), '议题内容')
  await user.click(screen.getByRole('button', { name: '创建议题' }))
  await screen.findByRole('heading', { name: '讨论知识' })
  await user.type(screen.getByLabelText('评论内容'), '保留评论草稿')
  await user.click(screen.getByRole('button', { name: '发表评论' }))
  await screen.findByText('当前账户没有执行此操作的权限。')
  expect(screen.getByLabelText('评论内容')).toHaveValue('保留评论草稿')
  await user.type(screen.getByLabelText('关联同一页面的提案 ID'), proposalID)
  await user.click(screen.getByRole('button', { name: '关联提案' }))
  await screen.findByText(/版本已变化/)
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path.endsWith('/proposals') && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string)).toEqual({ proposal_id: proposalID, expected_version: 4 })
})
it('creates a media page only after reserving and uploading original bytes', async () => {
  const uploadPath = `/api/v1/uploads/${proposalID}/content`
  const { user, fetcher } = mount('/pages/new', (path, init) => {
    if (path === '/api/v1/uploads' && init.method === 'POST')
      return json(
        {
          id: pageID,
          state: 'pending',
          upload_path: uploadPath,
          expires_at: revision.created_at,
          blob_id: null,
        },
        201,
      )
    if (path === uploadPath && init.method === 'PUT')
      return json({ id: revisionID, size: 3, mime: 'image/png', sha256: 'a'.repeat(64) })
    if (path === '/api/v1/pages' && init.method === 'POST') return failure('validation_failed', 422)
  })
  await screen.findByRole('heading', { name: '创建页面' })
  await user.click(screen.getByLabelText('页面类型'))
  await user.click(screen.getByRole('option', { name: '媒体' }))
  await user.type(screen.getByLabelText('标题'), '图片知识')
  expect(screen.getByRole('button', { name: '创建并发布' })).toBeDisabled()
  await user.upload(
    screen.getByLabelText('上传媒体原件'),
    new File(['png'], 'a.png', { type: 'image/png' }),
  )
  await screen.findByText('原件上传完成。')
  await user.click(screen.getByRole('button', { name: '创建并发布' }))
  await screen.findByText(/内容未通过校验/)
  const [, init] = fetcher.mock.calls.find(
    ([path, init]) => path === '/api/v1/pages' && init.method === 'POST',
  )!
  expect(JSON.parse(init.body as string).snapshot).toMatchObject({
    type: 'media',
    title: '图片知识',
    blob_id: revisionID,
    description: '',
  })
})

it('keeps IME confirmation inside the tag input and includes pending unsplit tags on submit', async () => {
  let posted: Schema<'Snapshot'> | undefined
  const { user } = mount('/pages/new', (path, init) => {
    if (path === '/api/v1/pages' && init.method === 'POST') {
      posted = JSON.parse(String(init.body)).snapshot
      return json({ page: detail.page, revision })
    }
  })
  await user.type(await screen.findByRole('textbox', { name: '标题' }), '标签测试')
  const input = screen.getByRole('textbox', { name: '标签' })
  fireEvent.compositionStart(input)
  fireEvent.change(input, { target: { value: '中文标签' } })
  fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', isComposing: true })
  expect(screen.queryByRole('button', { name: '移除标签 中文标签' })).not.toBeInTheDocument()
  expect(posted).toBeUndefined()
  fireEvent.compositionEnd(input)
  await user.click(input)
  await user.keyboard('{Enter}')
  await user.click(screen.getByRole('button', { name: '移除标签 中文标签' }))
  await user.type(input, 'a,b c')
  await user.click(screen.getByRole('button', { name: '创建并发布' }))
  await waitFor(() => expect(posted?.tags).toEqual(['a,b c']))
})
it('requires confirmation before leaving an unsaved editor', async () => {
  const { user } = mount('/pages/new')
  await user.type(await screen.findByRole('textbox', { name: '标题' }), '未提交')
  await user.click(screen.getByRole('link', { name: '全部页面' }))
  await screen.findByRole('alertdialog')
  await user.click(screen.getByRole('button', { name: '继续编辑' }))
  expect(screen.getByRole('textbox', { name: '标题' })).toHaveValue('未提交')
  await user.click(screen.getByRole('link', { name: '全部页面' }))
  await user.click(await screen.findByRole('button', { name: '放弃修改' }))
  await screen.findByRole('heading', { name: '全部页面' })
})
it('does not expose a proposal through another page route', async () => {
  mount(`/pages/99/proposals/${proposalID}`)
  await screen.findByText('未找到此提案。')
  expect(screen.queryByRole('button', { name: '合并' })).not.toBeInTheDocument()
  expect(screen.queryByText(proposal.current_revision.snapshot.title)).not.toBeInTheDocument()
})

it('keeps focus while typing a multi-character tag filter into the URL-backed directory', async () => {
  const { user, fetcher } = mount('/pages')
  const input = await screen.findByRole('textbox', { name: '标签筛选' })
  await user.type(input, 'knowledge')
  expect(input).toHaveValue('knowledge')
  expect(input).toHaveFocus()
  await waitFor(() =>
    expect(fetcher.mock.calls.some(([url]) => url.includes('tag=knowledge'))).toBe(true),
  )
})

it('paginates group member choices without submitting the group form', async () => {
  const second = { ...account, id: proposalID, username: 'bob', display_name: 'Bob' }
  const { user, fetcher } = mount('/admin/groups', (path, init) => {
    if (path.startsWith('/api/v1/admin/groups?'))
      return json(
        list([
          { id: proposalID, name: '成员分页', version: 1, member_ids: [], create_page: false },
        ]),
      )
    if (path.startsWith('/api/v1/admin/accounts?'))
      return path.includes('cursor=next-members')
        ? json(list([{ account: second, auth_version: 1 }]))
        : json({ items: [{ account, auth_version: 1 }], next_cursor: 'next-members' })
    if (init.method === 'PUT') return failure('forbidden', 403)
  })
  await user.click(await screen.findByRole('checkbox', { name: /Alice/ }))
  await user.click(screen.getByRole('button', { name: /下一页/ }))
  await screen.findByRole('checkbox', { name: /Bob/ })
  await user.click(screen.getByRole('button', { name: /上一页/ }))
  expect(await screen.findByRole('checkbox', { name: /Alice/ })).toBeChecked()
  expect(fetcher.mock.calls.some(([, init]) => init.method === 'PUT')).toBe(false)
  await user.click(screen.getByRole('button', { name: '保存用户组' }))
  await waitFor(() =>
    expect(fetcher.mock.calls.filter(([, init]) => init.method === 'PUT')).toHaveLength(1),
  )
})

it('labels rebase conflict sides according to the Core contract', async () => {
  const { user } = mount(`/pages/${pageID}/proposals/${proposalID}`, (path, init) => {
    if (path.endsWith('/diff'))
      return json({
        base_revision_id: candidateID,
        head_revision_id: revisionID,
        proposal_revision_id: candidateID,
        stale: true,
        fields: [],
      })
    if (path.endsWith('/rebase') && init.method === 'POST')
      return failure('merge_conflict', 409, {
        conflicts: [{ path: '/markdown', base: 'BASE', ours: 'PUBLISHED', theirs: 'CANDIDATE' }],
      })
  })
  await user.click(await screen.findByRole('button', { name: '更新基础版本' }))
  const current = await screen.findByText('当前发布', { exact: true })
  expect(within(current).getByText('PUBLISHED')).toBeInTheDocument()
  expect(
    within(screen.getByText('待合入内容', { exact: true })).getByText('CANDIDATE'),
  ).toBeInTheDocument()
})

it('requires reading and acknowledging the current head before saving a stale candidate', async () => {
  const head = {
    ...revision,
    id: proposalID,
    snapshot: { ...revision.snapshot, type: 'document', markdown: '当前发布的新内容' },
  }
  const { user, fetcher } = mount(`/pages/${pageID}/proposals/${proposalID}/edit`, (path, init) => {
    if (path === `/api/v1/pages/${pageID}`)
      return json({
        ...detail,
        page: { ...detail.page, head_revision_id: head.id },
        revision: head,
      })
    if (path.endsWith('/revisions') && init.method === 'POST') return failure('stale_revision', 409)
  })
  const editor = await screen.findByLabelText('Markdown 正文')
  expect(await screen.findByRole('region', { name: '基础版本' })).toHaveTextContent(
    '安全的页面正文',
  )
  expect(screen.getByRole('region', { name: '当前发布版本' })).toHaveTextContent('当前发布的新内容')
  expect(screen.getByRole('button', { name: '保存修订' })).toBeDisabled()
  fireEvent.submit(editor.closest('form')!)
  expect(fetcher.mock.calls.some(([, init]) => init.method === 'POST')).toBe(false)
  await user.clear(editor)
  await user.type(editor, '已保留两边修改')
  await user.click(screen.getByRole('checkbox', { name: '已结合当前发布内容解决冲突' }))
  await user.click(screen.getByRole('button', { name: '保存修订' }))
  await screen.findByText(/版本已变化/)
  expect(editor).toHaveValue('已保留两边修改')
  const [, init] = fetcher.mock.calls.find(([, init]) => init.method === 'POST')!
  expect(JSON.parse(String(init.body))).toMatchObject({
    expected_head_revision_id: head.id,
    expected_proposal_revision_id: candidateID,
    snapshot: { markdown: '已保留两边修改' },
  })
})

it('requires displaying the diff before merging from a preview deep link', async () => {
  const { user } = mount(`/pages/${pageID}/proposals/${proposalID}?view=preview`)
  expect(await screen.findByRole('button', { name: '合并' })).toBeDisabled()
  await user.click(screen.getByRole('tab', { name: '变更对比' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '合并' })).toBeEnabled())
  await user.click(screen.getByRole('tab', { name: '候选历史' }))
  expect(screen.getByRole('button', { name: '合并' })).toBeDisabled()
})

it('moves focus to the selected article heading after closing the mobile contents sheet', async () => {
  const { user } = mount('/pages/2')
  const heading = await screen.findByRole('heading', { name: '内容', level: 2 })
  const scroll = vi.spyOn(heading, 'scrollIntoView')
  const trigger = screen.getByRole('button', { name: '文章目录' })
  await user.click(trigger)
  await user.click(within(screen.getByRole('dialog')).getByRole('link', { name: '内容' }))
  await waitFor(() => expect(heading).toHaveFocus())
  expect(scroll).toHaveBeenCalledWith({ block: 'start' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

  await user.click(trigger)
  await user.keyboard('{Escape}')
  await waitFor(() => expect(trigger).toHaveFocus())
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
