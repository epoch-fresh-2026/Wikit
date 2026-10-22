# Wikit

[中文](README.zh-CN.md) · [Design documentation](docs/README.md) · [Contributing](CONTRIBUTING.md)

Wikit is an open-source, self-hosted community wiki with Git-inspired proposals,
diffs, merges, and immutable publication history. Documents and media are
independent Pages organized with tags. Humans and agents share the same account
permissions through REST and MCP.

**Status: Go backend with accounts, permission governance, immutable history, atomic publication, three-way rebase and revert/restore proposals, backed by PostgreSQL
tests. Media uploads, authorized Range downloads and safe local GC are implemented.
Page Issues, immutable comments, proposal associations and audited moderation are implemented.
Chinese/English search, durable indexing tasks and full index rebuilds are implemented with Meilisearch.
The official Go SDK serves 39 authenticated MCP tools using the same Core.
The React WebUI provides reading, editing, merge, discussion and governance flows with frontend CI.
The v0.1.0 GHCR image has passed full release CI and acceptance after pulling its immutable digest;
production deployment remains manual.** See the [release evidence and registry access requirements](docs/release-readiness.md),
[implementation roadmap](docs/roadmap.md) and
[GitHub Issues](https://github.com/epoch-fresh-2026/Wikit/issues).

The Issue #35 redesign on this branch uses numeric Page IDs, a fixed home Page,
explicit create_page grants, direct first publication, and a Wiki SPA with separate admin navigation.
It is a fresh-deployment schema and is not included in the older image above. Existing UUID databases
are rejected without deletion; rollback requires the matching old image and backup.
Account IDs also use database-generated positive bigint values, serialized as decimal strings; this is a breaking fresh-deployment baseline with no UUID data migration ([ADR 0023](docs/adr/0023-numeric-account-ids.md)).
See [redesign validation](docs/redesign-validation.md).

## Architecture

React / TypeScript / Vite / shadcn/ui → REST; agents → MCP Streamable HTTP.
Both adapters use one Go Core. PostgreSQL owns business state, a local persistent
volume stores immutable media, and Meilisearch provides rebuildable Chinese and
English search. Git is not a runtime storage dependency.

The v0.1 target is a single-host Docker Compose deployment. Public and restricted
Pages, per-Page permissions, single-Page proposals, version-pinned media references,
merges, restore/revert proposals, and Page Issues are specified in the
[design](docs/design.md). Native Git access, multi-tenancy, templates, collaborative
editing, notifications, semantic search, S3, and high availability are outside v0.1.

## Work on the design

Install Node.js 24, then run:

```sh
npm ci
npm run check
```

These commands validate documentation, API and MCP contracts, and repository
workflow configuration. They do not test a working wiki. Application checks become
mandatory as implementation stages are enabled. See [development](docs/development.md)
and the [deployment specification](docs/operations.md) for planned runtime setup.

For the WebUI, run `npm ci --prefix web`, then `npm run dev --prefix web`.
See [WebUI development and state boundaries](docs/web.md) for the backend origin configuration and checks.

## Contribute

Confirm the design and acceptance criteria in a GitHub Issue, update affected
documents before code, and submit a linked PR. Code must be highly cohesive,
loosely coupled, modular, and extensible through concrete boundaries; see
[coding standards](docs/coding-standards.md). Detailed design documents are in Chinese;
API names and code identifiers are English.

## License

Project source is licensed under [MIT](LICENSE). A deployed site's user-contributed
content has its own policy; the source license does not automatically license it.
