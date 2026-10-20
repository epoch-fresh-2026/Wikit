import { mkdirSync, writeFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const string = (maxLength = 4096) => ({ type: 'string', maxLength });
const id = { type: 'string', format: 'uuid' };
const nullableId = { type: ['string', 'null'], format: 'uuid' };
const integer = { type: 'integer', minimum: 0 };
const date = { type: 'string', format: 'date-time' };
const mediaTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp',
  'audio/mpeg', 'audio/wav', 'audio/ogg', 'video/mp4', 'video/webm', 'application/pdf'];
const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const array = (items) => ({ type: 'array', items });
const enumOf = (...values) => ({ type: 'string', enum: values });
const object = (properties, required = Object.keys(properties)) => ({
  type: 'object', additionalProperties: false, required, properties,
});
const page = (name) => object({
  items: array(ref(name)), next_cursor: { type: ['string', 'null'] },
});
const expected = {
  expected_head_revision_id: nullableId,
  expected_proposal_revision_id: id,
};
const snapshotBase = {
  title: { ...string(300), minLength: 1 },
  tags: { ...array({ ...string(64), minLength: 1 }), maxItems: 30, uniqueItems: true },
};
const schemas = {
  Account: object({
    id, username: string(32), display_name: string(100),
    role: enumOf('member', 'reviewer', 'admin'), status: enumOf('active', 'disabled'),
  }),
  RegisterInput: object({
    username: { ...string(32), minLength: 3, pattern: '^[A-Za-z0-9_]+$' },
    password: { ...string(128), minLength: 12, writeOnly: true },
  }),
  LoginInput: object({ username: string(32), password: { ...string(128), writeOnly: true } }),
  PasswordInput: object({ current_password: { ...string(128), writeOnly: true } }),
  PasswordChange: object({
    current_password: { ...string(128), writeOnly: true },
    new_password: { ...string(128), minLength: 12, writeOnly: true },
  }),
  Session: object({ account: ref('Account'), csrf_token: string(256) }),
  Registration: object({ account: ref('Account'), token: string(256) }),
  Token: object({ token: string(256), generation: integer, created_at: date }),
  TokenStatus: object({ active: { type: 'boolean' }, generation: integer, created_at: date }),
  AccountUpdate: object({
    role: enumOf('member', 'reviewer', 'admin'),
    status: enumOf('active', 'disabled'), expected_auth_version: integer,
  }),
  AccountAdmin: object({ account: ref('Account'), auth_version: integer }),
  Group: object({ id, name: string(100), version: integer, member_ids: array(id) }),
  GroupCreate: object({ name: { ...string(100), minLength: 1 } }),
  GroupUpdate: object({ name: string(100), member_ids: array(id), expected_version: integer }),
  DocumentSnapshot: object({
    type: { const: 'document', type: 'string' }, ...snapshotBase, markdown: string(1048576),
  }),
  MediaSnapshot: object({
    type: { const: 'media', type: 'string' }, ...snapshotBase,
    description: string(65536), blob_id: id,
  }),
  Snapshot: { oneOf: [ref('DocumentSnapshot'), ref('MediaSnapshot')] },
  Page: object({
    id, type: enumOf('document', 'media'), visibility: enumOf('public', 'restricted'),
    status: enumOf('proposed', 'published', 'hidden'),
    head_revision_id: nullableId, policy_version: integer,
  }),
  Revision: object({
    id, page_id: id, parent_id: nullableId, source_proposal_id: id,
    snapshot: ref('Snapshot'), created_at: date, author_id: id,
    content_url: { type: ['string', 'null'] },
  }),
  PageDetail: object({
    page: ref('Page'), revision: { anyOf: [ref('Revision'), { type: 'null' }] },
  }),
  ProposalRevision: object({
    id, proposal_id: id, base_revision_id: nullableId,
    snapshot: ref('Snapshot'), created_at: date,
  }),
  Proposal: object({
    id, page_id: id, author_id: id, state: enumOf('open', 'merged', 'closed'),
    version: integer, review_round: integer, current_revision: ref('ProposalRevision'),
  }),
  CreatePage: object({ visibility: enumOf('public', 'restricted'), snapshot: ref('Snapshot') }),
  CreatedPage: object({ page: ref('Page'), proposal: ref('Proposal') }),
  CreateProposal: object({ expected_head_revision_id: nullableId, snapshot: ref('Snapshot') }),
  UpdateProposal: object({ ...expected, snapshot: ref('Snapshot') }),
  VersionPair: object(expected),
  ProposalState: object({
    state: enumOf('open', 'closed'), expected_proposal_version: integer,
  }),
  ReviewInput: object({
    ...expected, expected_review_round: integer,
    decision: enumOf('approve', 'request_changes', 'comment'), body: string(65536),
  }),
  Review: object({
    id, actor_id: id, proposal_revision_id: id, base_revision_id: nullableId,
    round: integer, decision: enumOf('approve', 'request_changes', 'comment'),
    body: string(65536), created_at: date,
  }),
  RollbackInput: object({ revision_id: id, expected_head_revision_id: id }),
  Grant: object({
    subject_type: enumOf('user', 'group'), subject_id: id,
    capabilities: {
      ...array(enumOf('read', 'propose', 'discuss', 'review', 'manage')), uniqueItems: true,
    },
  }),
  Policy: object({
    visibility: enumOf('public', 'restricted'), grants: array(ref('Grant')), policy_version: integer,
  }),
  PolicyInput: object({
    visibility: enumOf('public', 'restricted'), grants: array(ref('Grant')),
    expected_policy_version: integer,
  }),
  ModerationInput: object({
    hidden: { type: 'boolean' }, reason: { ...string(2000), minLength: 1 }, expected_version: integer,
  }),
  Issue: object({
    id, page_id: id, author_id: id, title: string(300), body: string(65536),
    state: enumOf('open', 'closed'), hidden: { type: 'boolean' }, version: integer,
    proposal_ids: array(id), created_at: date,
  }),
  IssueInput: object({ title: { ...string(300), minLength: 1 }, body: string(65536) }),
  IssueState: object({ state: enumOf('open', 'closed'), expected_version: integer }),
  IssueLink: object({ proposal_id: id, expected_version: integer }),
  CommentInput: object({ body: { ...string(65536), minLength: 1 } }),
  Comment: object({
    id, issue_id: id, author_id: id, body: string(65536), hidden: { type: 'boolean' },
    version: integer, created_at: date,
  }),
  UploadInput: object({
    filename: { ...string(255), minLength: 1 }, size: { type: 'integer', minimum: 1 },
    mime: enumOf(...mediaTypes),
  }),
  Upload: object({
    id, state: enumOf('pending', 'ready', 'failed'), upload_path: string(),
    expires_at: date, blob_id: nullableId,
  }),
  Blob: object({
    id, size: integer, mime: string(100), sha256: { type: 'string', pattern: '^[a-f0-9]{64}$' },
  }),
  Diff: object({
    base_revision_id: nullableId, head_revision_id: nullableId, proposal_revision_id: id,
    stale: { type: 'boolean' },
    fields: array(object({
      path: string(100), before: { type: ['string', 'null'] },
      after: { type: ['string', 'null'] }, unified_diff: { type: ['string', 'null'] },
    })),
  }),
  Tag: object({ key: string(64) }),
  SearchHit: object({
    page_id: id, revision_id: id, type: enumOf('document', 'media'),
    title: string(300), tags: array(string(64)), snippet: string(2000),
  }),
  AuditEvent: object({
    id, actor_id: nullableId, action: string(100), target_id: nullableId,
    request_id: string(100), created_at: date,
  }),
  Error: object({
    error: object({
      code: enumOf(
        'invalid_request', 'unauthenticated', 'forbidden', 'not_found', 'stale_revision',
        'merge_conflict', 'invalid_state', 'review_required', 'idempotency_conflict',
        'validation_failed', 'rate_limited', 'upload_too_large', 'unsupported_media',
        'search_unavailable', 'storage_unavailable', 'internal_error',
      ),
      message: string(), request_id: string(100),
      conflicts: array(object({
        path: string(100), base: { type: ['string', 'null'] },
        ours: { type: ['string', 'null'] }, theirs: { type: ['string', 'null'] },
      })),
    }, ['code', 'message', 'request_id']),
  }),
};
for (const name of [
  'AccountAdmin', 'Group', 'PageDetail', 'Revision', 'Proposal', 'ProposalRevision',
  'Review', 'Issue', 'Comment', 'Tag', 'SearchHit', 'AuditEvent',
]) schemas[`${name}List`] = page(name);

const operations = [];
const implementedRest = new Set([
  'register', 'login', 'logout', 'account_me', 'password_change', 'token_status',
  'token_rotate', 'token_revoke', 'accounts_list', 'account_update', 'groups_list',
  'group_create', 'group_update', 'audit_list', 'page_policy_get', 'page_policy_update',
  'page_create', 'pages_list', 'page_get', 'revisions_list', 'revision_get',
  'proposal_queue', 'proposals_list', 'proposal_get', 'proposal_revisions_list', 'tags_list',
  'proposal_create', 'proposal_update', 'proposal_diff', 'proposal_rebase', 'reviews_list',
  'review_create', 'proposal_merge', 'proposal_state', 'page_revert', 'page_restore',
  'upload_prepare', 'upload_content', 'upload_get', 'revision_content', 'proposal_content',
  'search', 'page_moderate', 'issues_list', 'issue_create', 'issue_get', 'issue_state',
  'issue_link_proposal', 'issue_moderate', 'comments_list', 'comment_create', 'comment_moderate',
]);
function op(method, path, name, tag, summary, output, input, options = {}) {
  operations.push({ method, path, name, tag, summary, output, input, ...options });
}
const anonymous = { anonymous: true, mcp: false, sensitive: true };
const sessionOnly = { sessionOnly: true, mcp: false, sensitive: true };
op('post', '/auth/register', 'register', 'Accounts', '注册账户并显示一次 Token', 'Registration', 'RegisterInput', { ...anonymous, status: 201 });
op('post', '/auth/login', 'login', 'Accounts', '登录并建立安全会话', 'Session', 'LoginInput', anonymous);
op('post', '/auth/logout', 'logout', 'Accounts', '注销当前会话', null, null, sessionOnly);
op('get', '/auth/me', 'account_me', 'Accounts', '获取当前账户', 'Account', null, { mcp: false });
op('post', '/auth/password', 'password_change', 'Accounts', '修改密码并撤销会话', null, 'PasswordChange', sessionOnly);
op('get', '/auth/token', 'token_status', 'Accounts', '查询账户 Token 状态', 'TokenStatus', null, sessionOnly);
op('post', '/auth/token', 'token_rotate', 'Accounts', '轮换唯一 Token', 'Token', 'PasswordInput', sessionOnly);
op('post', '/auth/token/revoke', 'token_revoke', 'Accounts', '撤销账户 Token', null, 'PasswordInput', sessionOnly);
op('get', '/admin/accounts', 'accounts_list', 'Administration', '管理员列出账户', 'AccountAdminList', null, { list: true });
op('patch', '/admin/accounts/{account_id}', 'account_update', 'Administration', '管理员更新角色或状态', 'AccountAdmin', 'AccountUpdate');
op('get', '/admin/groups', 'groups_list', 'Administration', '管理员列出用户组', 'GroupList', null, { list: true });
op('post', '/admin/groups', 'group_create', 'Administration', '管理员创建用户组', 'Group', 'GroupCreate', { status: 201 });
op('put', '/admin/groups/{group_id}', 'group_update', 'Administration', '管理员更新组及成员', 'Group', 'GroupUpdate');
op('get', '/admin/audit', 'audit_list', 'Administration', '管理员查询审计事件', 'AuditEventList', null, { list: true });
op('get', '/pages', 'pages_list', 'Pages', '列出当前可读已发布页面', 'PageDetailList', null, { public: true, list: true, query: { tag: string(64), type: enumOf('document', 'media') } });
op('post', '/pages', 'page_create', 'Pages', '创建页面及首个提案', 'CreatedPage', 'CreatePage', { status: 201 });
op('get', '/pages/{page_id}', 'page_get', 'Pages', '读取页面当前发布内容', 'PageDetail', null, { public: true });
op('get', '/pages/{page_id}/policy', 'page_policy_get', 'Pages', '管理者读取页面权限', 'Policy');
op('put', '/pages/{page_id}/policy', 'page_policy_update', 'Pages', '管理者替换页面授权策略', 'Policy', 'PolicyInput');
op('post', '/pages/{page_id}/moderation', 'page_moderate', 'Pages', '管理者隐藏或恢复页面', 'Page', 'ModerationInput');
op('get', '/pages/{page_id}/revisions', 'revisions_list', 'Pages', '列出页面发布历史', 'RevisionList', null, { public: true, list: true });
op('get', '/pages/{page_id}/revisions/{revision_id}', 'revision_get', 'Pages', '读取明确发布版本', 'Revision', null, { public: true });
op('get', '/pages/{page_id}/revisions/{revision_id}/content', 'revision_content', 'Media', '鉴权后读取固定版本原件', 'binary', null, { public: true, mcp: false, query: { download: enumOf('1') } });
op('get', '/proposals', 'proposal_queue', 'Proposals', '列出可读的提案审核队列', 'ProposalList', null, { public: true, list: true, query: { state: enumOf('open', 'closed', 'merged') } });
op('get', '/pages/{page_id}/proposals', 'proposals_list', 'Proposals', '列出页面提案', 'ProposalList', null, { public: true, list: true });
op('post', '/pages/{page_id}/proposals', 'proposal_create', 'Proposals', '提交单页面修改提案', 'Proposal', 'CreateProposal', { status: 201 });
op('get', '/proposals/{proposal_id}', 'proposal_get', 'Proposals', '读取提案及当前候选', 'Proposal', null, { public: true });
op('get', '/proposals/{proposal_id}/revisions', 'proposal_revisions_list', 'Proposals', '读取不可变候选历史', 'ProposalRevisionList', null, { public: true, list: true });
op('post', '/proposals/{proposal_id}/revisions', 'proposal_update', 'Proposals', '提交候选更新或已解决冲突', 'Proposal', 'UpdateProposal');
op('get', '/proposals/{proposal_id}/revisions/{proposal_revision_id}/content', 'proposal_content', 'Media', '鉴权后读取候选媒体原件', 'binary', null, { public: true, mcp: false, query: { download: enumOf('1') } });
op('get', '/proposals/{proposal_id}/diff', 'proposal_diff', 'Proposals', '对比基础与候选并标记过期', 'Diff', null, { public: true });
op('post', '/proposals/{proposal_id}/rebase', 'proposal_rebase', 'Proposals', '三方合并至当前发布版本并使审核失效', 'Proposal', 'VersionPair');
op('get', '/proposals/{proposal_id}/reviews', 'reviews_list', 'Proposals', '列出审核记录', 'ReviewList', null, { public: true, list: true });
op('post', '/proposals/{proposal_id}/reviews', 'review_create', 'Proposals', '对明确版本提交审核', 'Review', 'ReviewInput', { status: 201 });
op('post', '/proposals/{proposal_id}/merge', 'proposal_merge', 'Proposals', '原子合并经有效审核的提案', 'Revision', 'VersionPair');
op('post', '/proposals/{proposal_id}/state', 'proposal_state', 'Proposals', '关闭或重开提案', 'Proposal', 'ProposalState');
op('post', '/pages/{page_id}/revert', 'page_revert', 'Pages', '为撤销一次发布创建提案', 'Proposal', 'RollbackInput', { status: 201 });
op('post', '/pages/{page_id}/restore', 'page_restore', 'Pages', '为恢复旧快照创建提案', 'Proposal', 'RollbackInput', { status: 201 });
op('get', '/pages/{page_id}/issues', 'issues_list', 'Discussions', '列出页面议题', 'IssueList', null, { public: true, list: true, query: { include_hidden: { type: 'boolean' }, state: enumOf('open', 'closed') } });
op('post', '/pages/{page_id}/issues', 'issue_create', 'Discussions', '创建页面议题', 'Issue', 'IssueInput', { status: 201 });
op('get', '/issues/{issue_id}', 'issue_get', 'Discussions', '读取页面议题', 'Issue', null, { public: true, query: { include_hidden: { type: 'boolean' } } });
op('post', '/issues/{issue_id}/state', 'issue_state', 'Discussions', '关闭或重开页面议题', 'Issue', 'IssueState');
op('post', '/issues/{issue_id}/proposals', 'issue_link_proposal', 'Discussions', '关联同页面提案', 'Issue', 'IssueLink');
op('post', '/issues/{issue_id}/moderation', 'issue_moderate', 'Discussions', '管理者隐藏或恢复议题', 'Issue', 'ModerationInput');
op('get', '/issues/{issue_id}/comments', 'comments_list', 'Discussions', '读取可见评论', 'CommentList', null, { public: true, list: true, query: { include_hidden: { type: 'boolean' } } });
op('post', '/issues/{issue_id}/comments', 'comment_create', 'Discussions', '追加议题评论', 'Comment', 'CommentInput', { status: 201 });
op('post', '/comments/{comment_id}/moderation', 'comment_moderate', 'Discussions', '管理者隐藏或恢复评论', 'Comment', 'ModerationInput');
op('post', '/uploads', 'upload_prepare', 'Media', '预约鉴权媒体上传', 'Upload', 'UploadInput', { status: 201 });
op('put', '/uploads/{upload_id}/content', 'upload_content', 'Media', '流式写入媒体原件', 'Blob', 'binary', { mcp: false });
op('get', '/uploads/{upload_id}', 'upload_get', 'Media', '上传者读取上传状态', 'Upload');
op('get', '/tags', 'tags_list', 'Search', '列出当前可读页面的标签', 'TagList', null, { public: true, list: true });
op('get', '/search', 'search', 'Search', '检索并校验当前权限及版本', 'SearchHitList', null, { public: true, list: true, query: { q: { ...string(500), minLength: 1 }, tag: string(64) }, requiredQuery: ['q'] });

function closure(schema) {
  const names = new Set();
  function visit(value) {
    if (!value || typeof value !== 'object') return;
    if (value.$ref?.startsWith('#/components/schemas/')) {
      const name = value.$ref.split('/').at(-1);
      if (!names.has(name)) {
        if (!schemas[name]) throw new Error(`Unknown schema ${name}`);
        names.add(name);
        visit(schemas[name]);
      }
    }
    for (const [key, child] of Object.entries(value)) if (key !== '$ref') visit(child);
  }
  visit(schema);
  return names;
}

function standalone(schema) {
  const names = closure(schema);
  const resolved = { ...schema };
  if (names.size) resolved.$defs = Object.fromEntries([...names].sort().map((n) => [n, schemas[n]]));
  return JSON.parse(JSON.stringify(resolved).replaceAll('#/components/schemas/', '#/$defs/'));
}

export function buildContracts() {
  const paths = {};
  const tools = [];
  const error = { $ref: '#/components/responses/BusinessError' };
  for (const o of operations) {
    const pathNames = [...o.path.matchAll(/\{([^}]+)\}/g)].map((m) => m[1]);
    const parameters = pathNames.map((name) => ({ name, in: 'path', required: true, schema: id }));
    const query = {
      ...(o.list ? { limit: { type: 'integer', minimum: 1, maximum: 100, default: 20 }, cursor: string(2048) } : {}),
      ...o.query,
    };
    for (const [name, schema] of Object.entries(query)) parameters.push({ name, in: 'query', required: o.requiredQuery?.includes(name) ?? false, schema });
    const mutating = !['get', 'head'].includes(o.method);
    const idempotent = mutating && !o.sensitive && o.input !== 'binary';
    if (idempotent) parameters.push({
      name: 'Idempotency-Key', in: 'header', required: true,
      description: '同账户和操作下 24 小时内保留；相同 key 不同请求返回 409。',
      schema: { type: 'string', minLength: 8, maxLength: 128 },
    });
    if (mutating && !o.anonymous) parameters.push({
      name: 'X-CSRF-Token', in: 'header', required: false,
      description: 'Cookie 会话变更请求必填；Bearer-only 请求不需要。',
      schema: string(256),
    });
    const status = o.output ? (o.status ?? 200) : 204;
    const outputSchema = o.output === 'binary' ? { type: 'string', format: 'binary' } : o.output ? ref(o.output) : null;
    const success = { description: o.output ? '操作成功' : '操作成功，无响应体' };
    if (outputSchema) success.content = Object.fromEntries(
      (o.output === 'binary' ? mediaTypes : ['application/json']).map((type) => [type, { schema: outputSchema }]),
    );
    const responses = { [status]: success, 400: error, 401: error, 403: error, 404: error, 409: error, 413: error, 415: error, 422: error, 429: error, 500: error, 503: error };
    if (o.output === 'binary') {
      responses[206] = { ...success, description: '满足 Range 请求的部分原件' };
      responses[416] = { description: 'Range 无法满足' };
      responses[304] = { description: '当前权限验证通过后的条件响应' };
    }
    const operation = {
      operationId: o.name, tags: [o.tag], summary: o.summary,
      description: `${o.summary}。${implementedRest.has(o.name) ? (o.mcp === false ? 'REST 已实现；此操作不提供 MCP 工具。' : 'REST 与 MCP 工具已实现。') : '目标行为尚未实现。'}业务权限与状态机见 docs/api.md 及 docs/permissions.md。`,
      security: o.anonymous ? [] : o.sessionOnly ? [{ session: [] }] : o.public ? [{}, { session: [] }, { bearer: [] }] : [{ session: [] }, { bearer: [] }],
      parameters, responses,
    };
    if (o.input) operation.requestBody = {
      required: true, content: Object.fromEntries(
        (o.input === 'binary' ? ['application/octet-stream', ...mediaTypes] : ['application/json'])
          .map((type) => [type, { schema: o.input === 'binary' ? { type: 'string', format: 'binary' } : ref(o.input) }]),
      ),
    };
    paths[o.path] ??= {};
    paths[o.path][o.method] = operation;
    if (o.mcp !== false) {
      const inputProperties = Object.fromEntries(pathNames.map((n) => [n, id]));
      Object.assign(inputProperties, query);
      const required = [...pathNames, ...(o.requiredQuery ?? [])];
      if (o.input) {
        inputProperties.body = ref(o.input);
        required.push('body');
      }
      if (idempotent) {
        inputProperties.idempotency_key = { type: 'string', minLength: 8, maxLength: 128 };
        required.push('idempotency_key');
      }
      tools.push({
        name: `wikit_${o.name}`, description: o.summary,
        inputSchema: standalone(object(inputProperties, required)),
        outputSchema: standalone(object(o.output ? { data: ref(o.output) } : { ok: { type: 'boolean', const: true } })),
        annotations: { readOnlyHint: !mutating, destructiveHint: mutating, idempotentHint: !mutating || idempotent, openWorldHint: false },
        'x-rest-operation': o.name,
      });
    }
  }
  const usedSchemas = closure({ paths, error: ref('Error') });
  const openapi = {
    openapi: '3.1.0',
    info: {
      title: 'Wikit REST API', version: '0.1.0',
      description: '已锁定的 v0.1 设计契约；账户、授权、页面/历史、提案审核合并/回滚、讨论、媒体上传/下载及搜索 REST 已实现；目录内 MCP 工具已实现。所有传输入口复用 Core。',
      license: { name: 'MIT', identifier: 'MIT' },
    },
    servers: [{ url: 'https://wiki.example.com/api/v1', description: '替换为自托管站点地址' }],
    tags: [...new Set(operations.map((o) => o.tag))].map((name) => ({ name, description: `${name} 用例` })),
    paths,
    components: {
      responses: {
        BusinessError: {
          description: '业务错误；不可读对象返回 not_found，不泄露内容。',
          content: { 'application/json': { schema: ref('Error') } },
        },
      },
      securitySchemes: {
        bearer: { type: 'http', scheme: 'bearer', description: '账户唯一有效 Token' },
        session: { type: 'apiKey', in: 'cookie', name: 'wikit_session', description: '服务端安全会话，写操作必须校验 CSRF' },
      },
      schemas: Object.fromEntries([...usedSchemas].sort().map((name) => [name, schemas[name]])),
    },
  };
  return { openapi, mcp: { schema_version: 1, transport: 'streamable-http', endpoint: '/mcp', tools } };
}

export function serialize(value) { return `${JSON.stringify(value, null, 2)}\n`; }

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  mkdirSync('api', { recursive: true });
  const { openapi, mcp } = buildContracts();
  writeFileSync('api/openapi.json', serialize(openapi));
  writeFileSync('api/mcp-tools.json', serialize(mcp));
  console.log(`Generated ${operations.length} REST operations and ${mcp.tools.length} MCP tools.`);
}
