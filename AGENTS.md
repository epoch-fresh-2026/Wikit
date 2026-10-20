# Wikit contributor instructions

- Read `CONTEXT.md`, `docs/design.md`, and the affected contract before changing behavior.
- Work from a GitHub Issue with confirmed design and acceptance criteria. Update
  affected documentation before implementation; no separate design PR is required.
- Preserve the locked v0.1 scope. Record changes to consequential decisions in an ADR.
- Follow `docs/coding-standards.md`: high cohesion, low coupling, clear modules,
  and extension through small consumer-owned interfaces.
- Keep business rules and authorization in Core. REST/MCP must not duplicate them.
- Never claim that specified functionality or an unrun check is implemented.
- Run `npm run check` and the enabled implementation checks before submitting.
- Do not disable a CI stage to make a failing implementation pass.

## Current documentation

Use Context7 MCP whenever working with library, framework, SDK, API, CLI, or cloud
service syntax/configuration/setup/debugging. Resolve the official library name
first, select the relevant authoritative result, then query one concept at a time.
Prefer Context7 to web search for library documentation. General programming,
domain logic, refactoring, and code review do not require Context7.
