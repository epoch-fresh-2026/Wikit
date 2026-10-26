# Wikit contributor instructions

## Read before changing behavior

Read `CONTEXT.md`, `docs/requirements.md`, `docs/business-rules.md`,
`docs/architecture/overview.md`, `docs/architecture/modules.md`, and the affected interface contract.
Use `docs/README.md` to find the document that owns each concern.

## Issue-driven development

Every development task follows **Requirements Analysis → Issue → Design → Documentation → Implementation → PR → Review → Merge**
（需求分析 → 提 Issue → 设计 → 更新文档 → 开发 → 提 PR → 审查 → 合并）.
This applies to features, fixes, refactoring, documentation and dependency maintenance.
Unless the user explicitly limits the scope, carry the task through review, required checks, merge and Issue closure.
Do not stop after local edits, a commit, a push or PR creation and wait for the user to request the next stage.
Existing user authorization and confirmed requirements remain valid; do not request the same approval again.
If a real blocker prevents completion, report the exact failed stage, evidence and required action; never bypass checks or claim completion.

1. **Requirements Analysis**: understand the problem, users, expected behavior, scope, exclusions and acceptance criteria.
   Read the existing requirements, business rules, affected modules and contracts; inspect implementation and dependencies.
   Resolve consequential ambiguity before designing or changing behavior.
2. **Issue**: create or claim a GitHub Issue before editing specifications or implementation, recording the analysis,
   scope, dependencies and testable acceptance criteria. Continue an existing Issue for follow-up work within its scope.
   The Issue links to the relevant requirements, business rules, module design, interface and test sections.
   Keep task progress, discussion and delivery evidence in the Issue and PR, not in specification documents.
3. **Design**: define module ownership and collaboration, interfaces, data/transaction boundaries, authorization,
   failure handling and verification. Obtain maintainer confirmation and record it with `design-approved` before implementation;
   explicit requirements and decisions already confirmed by the maintainer count as confirmation.
4. **Documentation**: update the owning documents as coherent, complete descriptions of the confirmed design before implementation.
   Reconcile requirements, business rules, module collaboration, contracts and acceptance criteria, including their links.
   Record consequential choices and tradeoffs in `docs/architecture/decisions.md`. No separate design PR is required.
5. **Implementation**: implement the confirmed design and acceptance criteria, then run the affected checks.
   Commit and push the scoped changes to a working branch. If scope or design changes,
   update the Issue and owning documents and reconfirm the design before continuing dependent implementation.
6. **PR**: create a PR linking the same-repository Issue with `Closes #N` for complete delivery or `Refs #N` for partial delivery.
   Include `Design-Impact: updated`, or `Design-Impact: none` with a concrete `Design-Reason:`.
   Report actual checks, results and unrun checks; keep remaining acceptance items in the Issue.
7. **Review**: review the actual PR diff against requirements, business rules, module collaboration, interfaces, tests and documentation.
   Record review scope, findings and resolutions in the PR. Identify self-review honestly; do not claim independent approval.
   Fix findings, push corrections, re-review affected changes and wait for every enabled CI check on the latest head.
   Diagnose failed checks; never disable a stage, weaken assertions or repeatedly rerun unexplained failures to obtain a green result.
   Automated policy checks and passing CI do not replace substantive review.
8. **Merge**: after review feedback is resolved and all required checks pass on the latest head, merge through the protected PR workflow.
   Reconfirm the head commit before merging; do not bypass protections or self-approve where independent approval is required.
   Verify the PR is merged, update acceptance/evidence and confirm Issue closure when complete; partial deliveries leave it open.
   Synchronize the local checkout without overwriting unrelated changes, and report the Issue, PR, merge result and validation limits.

## Documentation maintenance

- Requirements belong in `docs/requirements.md`; business rules belong in `docs/business-rules.md`.
- Module ownership, dependencies and end-to-end collaboration belong in `docs/architecture/modules.md`.
- Edit the existing owning section; reconcile related sections and remove obsolete or duplicate statements.
  Do not append patch notes, dated updates, version-specific specifications or parallel redesign documents.
- Do not put development Issue numbers, Issue URLs or task histories in specifications.
  The link direction is Issue → document. Product Page Issues are a separate domain concept.
- Use Git for document history. Keep exact dependency versions in manifests/lockfiles and actual run evidence
  in PR/CI artifacts. Protocol identifiers, migration identifiers and immutable artifact references retain their technical meaning.
- Preserve the established product scope and package/contract release identifiers during documentation work.
- Update indexes, relative links, templates, generated references and check paths when moving files.

## Implementation and verification

- Follow `docs/engineering/coding-standards.md`: high cohesion, low coupling, clear modules,
  and extension through small consumer-owned interfaces.
- Keep business rules and authorization in Core. REST/MCP must not duplicate them.
- Never claim that specified functionality or an unrun check is implemented or verified.
- Run `npm run check` and the enabled implementation checks before submitting.
- Do not disable a CI stage to make a failing implementation pass.

## Current documentation

Use Context7 MCP whenever working with library, framework, SDK, API, CLI, or cloud
service syntax/configuration/setup/debugging. Resolve the official library name
first, select the relevant authoritative result, then query one concept at a time.
Prefer Context7 to web search for library documentation. General programming,
domain logic, refactoring, and code review do not require Context7.
