import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { repositoryFiles } from './repository-files.mjs';

test('repository checks include new project files and respect nested Git boundaries', () => {
  const root = mkdtempSync(join(tmpdir(), 'wikit-repository-check-'));
  try {
    for (const dir of ['docs', 'nested/.git', 'worktree', '.local', 'node_modules']) {
      mkdirSync(join(root, dir), { recursive: true });
    }
    writeFileSync(join(root, 'worktree/.git'), 'gitdir: elsewhere');
    for (const file of ['README.md', 'docs/new.md', 'nested/README.md', 'worktree/README.md', '.local/run.json', 'node_modules/package.json']) {
      writeFileSync(join(root, file), '');
    }
    assert.deepEqual(repositoryFiles(root).sort(), ['README.md', 'docs/new.md']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
