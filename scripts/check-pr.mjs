import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function parsePolicy(body, changedFiles) {
  const issueNumbers = [...body.matchAll(/\b(?:Closes|Fixes|Resolves)\s+#([1-9][0-9]*)\b/gi)]
    .map((m) => Number(m[1]));
  if (!issueNumbers.length) throw new Error('PR must link a same-repository Issue with Closes #123.');
  const impact = body.match(/^Design-Impact:\s*(updated|none)\s*$/m)?.[1];
  if (!impact) throw new Error('Set Design-Impact: updated or none.');
  if (impact === 'updated' && !changedFiles.some((p) =>
    p.startsWith('docs/') || p.startsWith('api/') || p === 'CONTEXT.md')) {
    throw new Error('Design-Impact: updated requires a design document or contract change.');
  }
  if (impact === 'none' && !/^Design-Reason:\s*\S.{9,}$/m.test(body)) {
    throw new Error('Design-Impact: none requires a concrete Design-Reason (at least 10 characters).');
  }
  return [...new Set(issueNumbers)];
}

async function checkPullRequest() {
  const eventPath = process.env.GITHUB_EVENT_PATH;
  if (!eventPath) {
    console.log('No GitHub event; PR policy is checked by CI.');
    return;
  }
  const event = JSON.parse(readFileSync(eventPath, 'utf8'));
  if (!event.pull_request) {
    console.log('Not a PR event; no PR metadata to validate.');
    return;
  }
  const repository = process.env.GITHUB_REPOSITORY;
  if (!/^[\w.-]+\/[\w.-]+$/.test(repository ?? '')) throw new Error('Invalid repository.');
  async function get(path) {
    const response = await fetch(`https://api.github.com/repos/${repository}/${path}`, {
      headers: {
        Accept: 'application/vnd.github+json',
        ...(process.env.GH_TOKEN ? { Authorization: `Bearer ${process.env.GH_TOKEN}` } : {}),
      },
    });
    if (!response.ok) throw new Error(`GitHub metadata request failed: ${response.status}`);
    return response.json();
  }
  // Fetch current metadata, so an edited PR description can be checked by a rerun.
  const pull = await get(`pulls/${event.pull_request.number}`);
  const files = [];
  for (let page = 1; ; page++) {
    const batch = await get(`pulls/${pull.number}/files?per_page=100&page=${page}`);
    files.push(...batch.map((f) => f.filename));
    if (batch.length < 100) break;
    if (page >= 30) throw new Error('PR exceeds metadata file limit; split the change.');
  }
  const numbers = parsePolicy(pull.body ?? '', files);
  for (const number of numbers) {
    const issue = await get(`issues/${number}`);
    if (issue.pull_request) throw new Error(`#${number} is a PR, not an Issue.`);
    if (!issue.labels.some((label) => label.name === 'design-approved')) {
      throw new Error(`Issue #${number} must have design-approved before implementation.`);
    }
  }
  console.log(`PR policy passed for Issue(s): ${numbers.join(', ')}.`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await checkPullRequest();
}
