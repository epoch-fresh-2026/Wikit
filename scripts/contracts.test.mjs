import test from 'node:test';
import assert from 'node:assert/strict';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { buildContracts } from './build-contracts.mjs';

const { openapi, mcp } = buildContracts();
const ajv = new Ajv2020({ strict: true });
addFormats(ajv);
const uuid = '00000000-0000-4000-8000-000000000001';
const validator = (name) => ajv.compile(mcp.tools.find((t) => t.name === name).inputSchema);

test('merge requires both version conditions and an idempotency key', () => {
  const validate = validator('wikit_proposal_merge');
  const input = { proposal_id: uuid, idempotency_key: 'request-123', body: { expected_head_revision_id: uuid, expected_proposal_revision_id: uuid } };
  assert(validate(input), JSON.stringify(validate.errors));
  delete input.body.expected_head_revision_id;
  assert(!validate(input));
  input.body.expected_head_revision_id = null;
  assert(validate(input), 'First publication can have a null head.');
  delete input.idempotency_key;
  assert(!validate(input));
});
test('single-Page content schema rejects mixed document and binary content', () => {
  const validate = validator('wikit_page_create');
  const input = { idempotency_key: 'request-123', body: { visibility: 'public', snapshot: { type: 'document', title: '知识库', tags: ['Wiki'], markdown: '# 内容' } } };
  assert(validate(input), JSON.stringify(validate.errors));
  input.body.snapshot.blob_id = uuid;
  assert(!validate(input));
});
test('search requires a query and rejects caller-provided authorization filters', () => {
  const validate = validator('wikit_search');
  assert(!validate({}));
  assert(validate({ q: '知识', limit: 20 }));
  assert(!validate({ q: '知识', allowed_user_ids: [uuid] }));
  assert(!validate({ q: '知识', limit: 101 }));
});
test('MCP has no password/token-management tool and every mapping exists', () => {
  const operationIds = new Set(Object.values(openapi.paths).flatMap((path) => Object.values(path).map((o) => o.operationId)));
  for (const tool of mcp.tools) {
    assert(operationIds.has(tool['x-rest-operation']));
    assert(!/password|token_rotate|token_revoke|login|register/.test(tool.name));
  }
});

test('tag filters use repeated REST arrays and equivalent MCP fields', () => {
  for (const [path, name] of [['/pages', 'wikit_pages_list'], ['/search', 'wikit_search']]) {
    const parameters = openapi.paths[path].get.parameters;
    const validate = validator(name);
    const base = name === 'wikit_search' ? { q: '备份' } : {};
    assert(validate({ ...base, tags: ['数据库', '部署'], tag_match: 'any', exclude_tags: ['过时'], type: 'document' }));
    assert(validate({ ...base, exclude_tags: ['过时'] }));
    assert(validate({ ...base, tag: '数据库' }));
    assert(!validate({ ...base, tag_match: 'or' }));
    assert(!validate({ ...base, tags: '数据库,部署' }));
    assert(!validate({ ...base, exclude_tags: [null] }));
    for (const field of ['tags', 'exclude_tags']) {
      const p = parameters.find((p) => p.name === field);
      assert.equal(p.style, 'form');
      assert.equal(p.explode, true);
      assert.deepEqual(p.schema, mcp.tools.find((t) => t.name === name).inputSchema.properties[field]);
    }
  }
});


test('session recovery requires Origin and Cookie but no prior CSRF or MCP tool', () => {
  const operation = openapi.paths['/auth/session'].post;
  assert.deepEqual(operation.security, [{ session: [] }]);
  assert(operation.parameters.some((p) => p.name === 'Origin' && p.required));
  assert(!operation.parameters.some((p) => p.name === 'X-CSRF-Token'));
  assert(!mcp.tools.some((t) => t['x-rest-operation'] === operation.operationId));
  const validate = ajv.compile(openapi.components.schemas.SessionRestoreInput);
  assert(validate({}));
  assert(!validate({ csrf_token: 'untrusted' }));
  assert(!validate(null));
});
