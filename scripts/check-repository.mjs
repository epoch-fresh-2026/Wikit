import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { dirname, resolve, relative } from 'node:path';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import YAML from 'yaml';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { buildContracts, serialize } from './build-contracts.mjs';

const root = process.cwd();
const read = (path) => readFileSync(path, 'utf8');
const ignored = new Set(['.git', 'node_modules', '.local', 'dist', 'coverage']);
function walk(dir = '.') {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (ignored.has(entry.name)) return [];
    const path = `${dir}/${entry.name}`;
    return entry.isDirectory() ? walk(path) : [path.replace(/^\.\//, '')];
  });
}
const files = walk();
let links = 0;
for (const file of files.filter((p) => p.endsWith('.md'))) {
  const content = read(file);
  assert(content.endsWith('\n'), `${file}: missing final newline`);
  for (const match of content.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g)) {
    const target = match[1].split(/\s+"/)[0];
    if (/^(https?:|mailto:|#)/.test(target)) continue;
    const local = decodeURIComponent(target.split('#')[0]);
    if (!local) continue;
    const path = resolve(dirname(file), local);
    assert(!relative(root, path).startsWith('..'), `${file}: link escapes repository: ${local}`);
    assert(existsSync(path), `${file}: broken link ${target}`);
    links++;
  }
}
for (const file of files.filter((p) => /\.ya?ml$/.test(p))) YAML.parse(read(file), { uniqueKeys: true });
for (const file of files.filter((p) => p.endsWith('.json'))) JSON.parse(read(file));

const { openapi, mcp } = buildContracts();
assert.equal(read('api/openapi.json'), serialize(openapi), 'OpenAPI is stale; run npm run contracts:build.');
assert.equal(read('api/mcp-tools.json'), serialize(mcp), 'MCP catalog is stale; run npm run contracts:build.');
const ajv = new Ajv2020({ strict: true });
addFormats(ajv);
const names = new Set();
for (const tool of mcp.tools) {
  assert(!names.has(tool.name), `Duplicate MCP tool: ${tool.name}`);
  names.add(tool.name);
  ajv.compile(tool.inputSchema);
  ajv.compile(tool.outputSchema);
}

const stages = JSON.parse(read('.github/implementation.json'));
const expectedKeys = ['backend', 'frontend', 'integration', 'container', 'release_ready'];
assert.deepEqual(Object.keys(stages).sort(), [...expectedKeys].sort(), 'Unknown/missing stage.');
for (const value of Object.values(stages)) assert.equal(typeof value, 'boolean');
const pathStages = {
  backend: ['go.mod'],
  frontend: ['web/package.json', 'web/package-lock.json'],
  integration: ['tests/integration'],
  container: ['Dockerfile', 'compose.yaml', 'scripts/smoke.sh'],
};
for (const [stage, paths] of Object.entries(pathStages)) {
  if (paths.some((p) => existsSync(p))) assert(stages[stage], `${stage}: implementation exists but check is disabled.`);
  if (stages[stage]) for (const path of paths) assert(existsSync(path), `${stage}: missing ${path}`);
}
if (files.some((p) => p.endsWith('.go'))) assert(stages.backend, 'Go files require backend checks.');
if (stages.backend) assert(files.some((p) => p.endsWith('_test.go')), 'Backend needs real tests.');
if (stages.frontend) {
  const pkg = JSON.parse(read('web/package.json'));
  for (const script of ['typecheck', 'test:ci', 'build']) assert(pkg.scripts?.[script], `Missing frontend ${script}.`);
}
if (stages.integration) {
  assert(stages.backend, 'Integration requires backend.');
  assert(files.some((p) => p.startsWith('tests/integration/') && p.endsWith('_test.go')), 'Integration suite missing.');
}
if (stages.container) assert(stages.backend && stages.frontend && stages.integration, 'Container stage requires application stages.');
if (stages.release_ready) {
  assert(Object.values(stages).every(Boolean), 'Release requires every stage.');
  assert(existsSync('docs/release-readiness.md'), 'Release readiness evidence missing.');
}
if (process.env.GITHUB_EVENT_PATH) {
  const event = JSON.parse(read(process.env.GITHUB_EVENT_PATH));
  const base = event.pull_request?.base?.sha;
  if (base) {
    assert(/^[a-f0-9]{40}$/.test(base), 'Invalid base SHA.');
    const previous = JSON.parse(execFileSync('git', ['show', `${base}:.github/implementation.json`], { encoding: 'utf8' }));
    for (const [key, enabled] of Object.entries(previous)) {
      assert(!enabled || stages[key], `Cannot disable previously enabled ${key}.`);
    }
  }
}

for (const file of files.filter((p) => p.startsWith('.github/workflows/') && /\.ya?ml$/.test(p))) {
  const content = read(file);
  const workflow = YAML.parse(content);
  assert(!workflow.on?.pull_request_target, `${file}: untrusted PR execution must not use pull_request_target.`);
  for (const match of content.matchAll(/^\s*-?\s*uses:\s*([^\s#]+)/gm)) {
    assert(match[1].startsWith('./') || /@[a-f0-9]{40}$/.test(match[1]), `${file}: action must be SHA-pinned: ${match[1]}`);
  }
}
console.log(`Repository checks passed: ${links} local links, ${Object.keys(openapi.paths).length} REST paths, ${mcp.tools.length} MCP tools.`);
console.log(`Implementation stages: ${JSON.stringify(stages)} (disabled stages are not application test passes).`);
