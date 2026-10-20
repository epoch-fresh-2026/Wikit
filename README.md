# Wikit

[中文](README.zh-CN.md) · [Design documentation](docs/README.md) · [Contributing](CONTRIBUTING.md)

Wikit is an open-source, self-hosted community wiki with Git-inspired proposals,
diff reviews, merges, and immutable publication history. Documents and media are
independent Pages organized with tags. Humans and agents share the same account
permissions through REST and MCP.

**Status: Go backend with accounts, permission governance, immutable history, reviewed
atomic publication, three-way rebase and revert/restore proposals, backed by PostgreSQL
tests. Uploads, MCP and deployment remain unimplemented; no production image has been released.** See the
[implementation roadmap](docs/roadmap.md) and
[GitHub Issues](https://github.com/epoch-fresh-2026/Wikit/issues).

## Architecture

React / TypeScript / Vite / shadcn/ui → REST; agents → MCP Streamable HTTP.
Both adapters use one Go Core. PostgreSQL owns business state, a local persistent
volume stores immutable media, and Meilisearch provides rebuildable Chinese and
English search. Git is not a runtime storage dependency.

The v0.1 target is a single-host Docker Compose deployment. Public and restricted
Pages, per-Page permissions, single-Page proposals, version-pinned media references,
reviews, restore/revert proposals, and Page Issues are specified in the
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

## Contribute

Confirm the design and acceptance criteria in a GitHub Issue, update affected
documents before code, and submit a linked PR. Code must be highly cohesive,
loosely coupled, modular, and extensible through concrete boundaries; see
[coding standards](docs/coding-standards.md). Detailed design documents are in Chinese;
API names and code identifiers are English.

## License

Project source is licensed under [MIT](LICENSE). A deployed site's user-contributed
content has its own policy; the source license does not automatically license it.
