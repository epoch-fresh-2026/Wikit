# Wikit

[中文](README.zh-CN.md) · [Design documentation](docs/README.md) · [Contributing](CONTRIBUTING.md)

Wikit is an open-source, self-hosted community wiki with Git-inspired proposals,
diffs, merges, and immutable publication history. Documents and media are
independent Pages organized with tags. Humans and agents share the same account
permissions through REST and MCP.

The source implements accounts and per-Page permissions, immutable history,
direct first publication, proposal merge/rebase/revert/restore, media upload and authorized downloads,
Page Issues and moderation, Chinese/English search, and index recovery. The Wiki WebUI and 39 MCP tools
use the same Core; MCP also provides bounded, cited RAG context from full-text search.

Page and Account IDs are database-generated positive integers serialized as exact decimal strings.
The home Page is fixed, page creation requires an explicit grant, and subsequent changes use proposals.
The current schema requires a fresh deployment and rejects old UUID databases without deleting data.
See the [operations guide](docs/engineering/operations.md) for deployment, compatibility, backup and recovery.

Current requirements and contracts are organized in the [documentation index](docs/README.md).
Release readiness must be verified against the exact source commit and image digest; historical checks do not certify current source.
See [release acceptance](docs/quality/release-readiness.md). Production deployment remains manual.

## Architecture

React / TypeScript / Vite / shadcn/ui → REST; agents → MCP Streamable HTTP.
Both adapters use one Go Core. PostgreSQL owns business state, a local persistent
volume stores immutable media, and Meilisearch provides rebuildable Chinese and
English search. Git is not a runtime storage dependency.

The deployment baseline is a single-host Docker Compose deployment. Public and restricted
Pages, per-Page permissions, single-Page proposals, version-pinned media references,
merges, restore/revert proposals, and Page Issues are specified in the
[requirements](docs/requirements.md). Native Git access, multi-tenancy, templates, collaborative
editing, notifications, semantic search, S3, and high availability are outside the product scope.

## Development checks

Install Node.js 24, then run:

```sh
npm ci
npm run check
```

These commands validate documentation, API and MCP contracts, and repository
workflow configuration. Backend, frontend, database integration and container checks
are also required; see [development](docs/engineering/development.md) and [testing](docs/quality/testing.md).
Runtime setup is described in the [operations guide](docs/engineering/operations.md).

For the WebUI, run `npm ci --prefix web`, then `npm run dev --prefix web`.
See [WebUI development and state boundaries](docs/interfaces/web.md) for the backend origin configuration and checks.

## Contribute

Confirm the design and acceptance criteria in a GitHub Issue, update affected
documents before code, and submit a linked PR. Code must be highly cohesive,
loosely coupled, modular, and extensible through concrete boundaries; see
[coding standards](docs/engineering/coding-standards.md). Detailed design documents are in Chinese;
API names and code identifiers are English.

## License

Project source is licensed under [MIT](LICENSE). A deployed site's user-contributed
content has its own policy; the source license does not automatically license it.
