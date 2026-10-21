import type { Schema } from '@/lib/api'
export const pageID = '11111111-1111-4111-8111-111111111111'
export const proposalID = '22222222-2222-4222-8222-222222222222'
export const revisionID = '33333333-3333-4333-8333-333333333333'
export const candidateID = '44444444-4444-4444-8444-444444444444'
export const account: Schema<'Account'> = {
  id: '55555555-5555-4555-8555-555555555555',
  username: 'alice',
  display_name: 'Alice',
  role: 'admin',
  status: 'active',
}
export const snapshot: Schema<'Snapshot'> = {
  type: 'document',
  title: '协作知识',
  tags: ['知识'],
  markdown: '## 内容\n安全的页面正文。',
}
export const revision: Schema<'Revision'> = {
  id: revisionID,
  page_id: pageID,
  parent_id: null,
  source_proposal_id: proposalID,
  snapshot,
  created_at: '2026-10-20T00:00:00Z',
  author_id: account.id,
  content_url: null,
}
export const detail: Schema<'PageDetail'> = {
  page: {
    id: pageID,
    type: 'document',
    visibility: 'public',
    status: 'published',
    head_revision_id: revisionID,
    policy_version: 2,
  },
  revision,
}
export const proposal: Schema<'Proposal'> = {
  id: proposalID,
  page_id: pageID,
  author_id: account.id,
  state: 'open',
  version: 3,
  review_round: 2,
  current_revision: {
    id: candidateID,
    proposal_id: proposalID,
    base_revision_id: revisionID,
    snapshot,
    created_at: '2026-10-20T00:00:00Z',
  },
}
export const list = <T>(items: T[]) => ({ items, next_cursor: null })
export function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
export function failure(code: string, status: number, extra = {}) {
  return json(
    {
      error: {
        code,
        message: 'private server diagnostics must not be shown',
        request_id: 'test-request-id',
        ...extra,
      },
    },
    status,
  )
}
