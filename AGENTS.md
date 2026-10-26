# Wikit contributor instructions

## Read before changing behavior

Read `CONTEXT.md`, `docs/requirements.md`, `docs/business-rules.md`,
`docs/architecture/overview.md`, `docs/architecture/modules.md`, and the affected interface contract.
Use `docs/README.md` to find the document that owns each concern.

## Issue-driven development

Follow **Issue → Design → Implementation → PR → Review → Merge**.

1. **Issue**: create or claim a GitHub Issue with the problem, scope, dependencies and testable acceptance criteria.
   The Issue links to the relevant requirements, business rules, module design, interface and test sections.
   Keep task progress, discussion and delivery evidence in the Issue and PR, not in specification documents.
2. **Design**: update the owning documents as coherent, complete descriptions of the intended behavior.
   Resolve cross-document inconsistencies and obtain maintainer confirmation with `design-approved` before implementation.
   No separate design PR is required. Record consequential choices and tradeoffs in `docs/architecture/decisions.md`.
3. **Implementation**: implement the confirmed design and acceptance criteria. If scope or design changes,
   update the Issue and owning documents and reconfirm the design before continuing dependent implementation.
4. **PR**: link the same-repository Issue with `Closes #N` for complete delivery or `Refs #N` for partial delivery.
   Include `Design-Impact: updated`, or `Design-Impact: none` with a concrete `Design-Reason:`.
   Report actual checks, results and unrun checks; keep remaining acceptance items in the Issue.
5. **Review**: check requirements, business rules, module collaboration, interfaces, tests and documentation together.
   Resolve review feedback and pass every enabled CI check. Automated policy checks do not replace design review.
6. **Merge**: merge only after review and required checks pass, using the repository's protected-branch rules.
   Close the Issue only when its acceptance criteria are complete; partial deliveries leave it open.

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
