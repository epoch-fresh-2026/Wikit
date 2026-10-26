import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const ignored = new Set(['.git', 'node_modules', '.local', 'dist', 'coverage']);

// Nested repositories have their own contracts and checks. Include ordinary
// untracked Wikit files, but do not descend into independent checkouts/worktrees.
export function repositoryFiles(root, directory = '') {
  return readdirSync(join(root, directory), { withFileTypes: true }).flatMap((entry) => {
    if (ignored.has(entry.name)) return [];
    const path = directory ? `${directory}/${entry.name}` : entry.name;
    if (!entry.isDirectory()) return [path];
    if (existsSync(join(root, path, '.git'))) return [];
    return repositoryFiles(root, path);
  });
}
