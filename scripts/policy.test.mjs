import test from 'node:test';
import assert from 'node:assert/strict';
import { parsePolicy } from './check-pr.mjs';

test('accepts confirmed-design declaration with a contract change', () => {
  assert.deepEqual(parsePolicy('Closes #12\nDesign-Impact: updated', ['api/openapi.json']), [12]);
});

test('accepts a partial delivery without automatically closing the Issue', () => {
  assert.deepEqual(parsePolicy('Refs #3\nDesign-Impact: updated', ['docs/interfaces/rest.md']), [3]);
});
test('requires a real local Issue reference rather than arbitrary URL text', () => {
  assert.throws(() => parsePolicy('See https://example.com/12\nDesign-Impact: updated', ['docs/architecture/overview.md']));
});
test('does not accept an updated-design claim without design changes', () => {
  assert.throws(() => parsePolicy('Fixes #2\nDesign-Impact: updated', ['internal/core/main.go']));
});
test('requires a meaningful explanation for a no-design-change fix', () => {
  assert.throws(() => parsePolicy('Closes #2\nDesign-Impact: none\nDesign-Reason: n/a', ['README.md']));
  assert.deepEqual(parsePolicy('Closes #2\nDesign-Impact: none\nDesign-Reason: Correct a typo without behavior changes.', ['README.md']), [2]);
});
