// Evaluation-only gold annotations are never passed to retrieval. Freeze this
// scorer before authoring a holdout; retain every mismatch in the output.
import fs from 'node:fs';
import assert from 'node:assert/strict';

const [fixturePath, resultPath, outputPath] = process.argv.slice(2);
assert(fixturePath && resultPath && outputPath, 'usage: node scripts/rag-requirement-audit.mjs FIXTURE RESULTS OUTPUT');
const fixture = JSON.parse(fs.readFileSync(fixturePath, 'utf8'));
const report = JSON.parse(fs.readFileSync(resultPath, 'utf8'));
const measurements = new Map(report.results.filter(r => r.track === 'rag_question').map(r => [r.id, r]));
const equalSet = (a, b) => JSON.stringify([...new Set(a)].sort()) === JSON.stringify([...new Set(b)].sort());
const rows = (fixture.cases ?? fixture.Cases).map(q => {
  const id = q.id ?? q.ID;
  const m = measurements.get(id);
  assert(m, `missing result ${id}`);
  const rag = m.rag, errors = [];
  if (q.expected_coverage && q.expected_coverage !== rag.coverage.status) errors.push('coverage_status');
  if (q.expected_bindings) {
    const key = r => JSON.stringify([r.subject,r.attribute,r.stage??null,[...(r.filters??[])].sort()]);
    if (!equalSet(q.expected_bindings.map(key),(rag.plan.bindings??[]).map(key))) errors.push('clause_bindings');
  }
  if (q.expected_requirements) {
    const expected = q.expected_requirements;
    if (!equalSet(expected.map(r => r.attribute), rag.plan.requirements.map(r => r.attribute))) errors.push('planned_attributes');
    const staged = expected.some(r => Object.hasOwn(r,'stage'));
    const key = r => JSON.stringify([r.subject, r.attribute, r.status,...(staged?[r.stage??null]:[])]);
    if (!equalSet(expected.map(key), rag.coverage.requirements.map(key))) errors.push('requirement_coverage');
  }
  for (const row of rag.coverage.requirements) {
    if (row.status === 'covered' && row.citation_ids.length === 0) errors.push('covered_without_citation');
    for (const id of row.citation_ids) {
      const e = rag.evidence.find(e => e.citation_id === id);
      if (!e || !row.page_ids.includes(e.page_id) || !m.hits.some(h => h.page_id === e.page_id && h.revision_id === e.revision_id)) errors.push('invalid_citation');
    }
  }
  if (rag.coverage.status === 'complete' && (rag.coverage.requirements.length === 0 || rag.coverage.requirements.some(r => r.status !== 'covered') || rag.coverage.truncated || rag.coverage.unsupported_constraints.length)) errors.push('unsafe_complete');
  return {id, passed: errors.length === 0, errors: [...new Set(errors)], expected_coverage: q.expected_coverage ?? null, expected_bindings:q.expected_bindings??null, expected_requirements: q.expected_requirements ?? null, result: m};
});
const out = {fixture: fixturePath, results: resultPath, total: rows.length, passed: rows.filter(r => r.passed).length, annotated: rows.filter(r => r.expected_requirements !== null).length, failures: rows.filter(r => !r.passed), rows};
fs.writeFileSync(outputPath, JSON.stringify(out, null, 2) + '\n');
console.log(JSON.stringify({total:out.total,passed:out.passed,annotated:out.annotated,failures:out.failures.map(r=>({id:r.id,errors:r.errors}))}));
