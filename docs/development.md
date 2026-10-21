# 开发工作流与 CI/CD

## 本轮与后续实现

Issue #2 建立后端基础：Go composition root、配置、健康检查、SQL 迁移和真实
PostgreSQL 测试。Issue #3 增加账户、会话、Token 和授权治理；Issue #4 增加页面
创建/读取、首提案和历史读取；Issue #5 增加审核、原子合并、三方 rebase 与撤销/恢复。
Issue #6 增加流式媒体上传、固定版本鉴权下载和 blob-gc；Issue #7 增加页面议题、评论、
同 Page 提案关联，以及带理由与审计的页面/讨论隐藏和恢复。可部署 v0.1 仍待后续实现。
Issue #10 启用 frontend CI；Issue #11 增加容器、恢复及基准验收；release_ready 在证据齐备前保持关闭。

WebUI 位于 web/，使用 React 19.3.1、TypeScript 5.9.3、Vite 8.3.4、Tailwind 4.3.3 与 shadcn/ui。
源码组件由 shadcn CLI 4.22.1 导入，使用本地 cn 工具；运行依赖由 package-lock.json 锁定。
API 类型由 openapi-typescript 7.13.0 生成，typecheck 先检查生成结果与契约一致。
测试使用 Vitest 5.0.5、Testing Library、jsdom 29.1.1（兼容本地 Node 24.14）。
命令：npm ci --prefix web、npm run typecheck --prefix web、npm run test:ci --prefix web、
npm run build --prefix web；开发代理与后端 origin 见 [WebUI](web.md)。

Issue #9 增加官方 Go MCP SDK v1.7.0 和 jsonschema-go v0.4.3，协议依赖只在适配器/组合根使用。
`serve` 同时提供 REST 和 /mcp；MCP 只使用用户 Bearer Token，空闲 session 15 分钟过期。
集成测试使用真实 SDK StreamableClientTransport，而非手写协议替代客户端；覆盖全部 41 工具、
REST/MCP 幂等/游标/错误等价，以及账户/组/页面撤权与真实合并的两种提交顺序。
MCP 不接收二进制工具参数；upload_prepare 返回同源上传路径，再用同一 Token PUT 原件。

Issue #8 增加 Meilisearch 1.55.0 的中英文搜索、可恢复 outbox worker 和 search-rebuild/retry/status。
本地测试额外要求 WIKIT_MEILI_URL、WIKIT_MEILI_KEY 指向专用 Meilisearch；测试使用独立索引，
不会接触线上索引。CI 在既有 integration 阶段启动真实 Meilisearch，不增加跳过路径。
可运行 `go run ./cmd/wikit search-rebuild` 重建，`search-status` 查看积压，`search-retry` 立即重试失败事件。

后端使用 Go 1.25.0、pgx v5.10.0、PostgreSQL 18.6（本地与 CI），依赖由
go.mod/go.sum 锁定。Argon2id 使用 golang.org/x/crypto v0.53.0，终端使用
golang.org/x/term v0.44.0（兼容 Go 1.25）；仅 composition root 装配实现。
Unicode 原语使用 x/text v0.38.0，CommonMark 解析使用 Goldmark v1.8.6。
逐行编辑使用 go-diff v1.4.0，第三方依赖只在文本适配器中使用。

```sh
go run ./cmd/wikit migrate
go run ./cmd/wikit create-admin
go run ./cmd/wikit serve
go run ./cmd/wikit blob-gc
go vet ./...
go test -race -count=1 ./...
go test -race -count=1 -tags=integration ./tests/integration/...
```

环境配置见 `.env.example`（命令读取进程环境，不自动加载 dotenv）。
serve 必须提供 WIKIT_PUBLIC_URL、WIKIT_DATABASE_URL；媒体目录必须已存在。
migrate 只要求 WIKIT_DATABASE_URL，迁移事务使用 60 秒超时；不会随 serve 自动执行。
集成测试要求 WIKIT_DATABASE_URL 指向允许 CREATE DATABASE 的测试账户；
每例创建独立数据库并在结束时删除，缺少配置直接失败，不跳过。
首次启动前必须完成迁移并交互创建管理员；serve 在没有活跃管理员时拒绝监听。
create-admin 只要求数据库配置，禁止密码参数或非终端输入。账户与授权端点范围见
API 文档，其他未知路由返回 404。搜索服务尚未接入健康状态。
会话 TTL 可配置为 5m–168h（默认 24h），注册通过 WIKIT_REGISTRATION_ENABLED 控制。

本地文档工具需要 Node.js 24 和 npm：

```sh
npm ci
npm run check
```

后端、前端、集成和容器阶段用 `.github/implementation.json` 显式登记。
全为 false 表示应用尚未实现。对应路径出现时检查会要求启用阶段；
阶段一旦启用，后续 PR 不允许回退。新增阶段必须带真实测试，不能靠空测试通过。
主分支初始只有文档／契约检查；不能把跳过的应用测试宣称为已验证。
`release_ready` 是额外发布门槛，只有完成全部产品验收并提交发布证据后才能启用。

## 工作流

Issue 模板包含范围、设计、验收、依赖和文档影响。维护者确认后加
`design-approved`。实现 PR 关联同仓库已确认 Issue，先改文档再代码。
完整交付用 `Closes #N`；分阶段交付用 `Refs #N` 并保留 Issue 内未完成验收项，
两种引用都必须通过设计确认检查，不能用关闭引用误关跨 Issue 的验收。
小修复允许 `Design-Impact: none`，但必须写原因；评审确认是否合理。
外部贡献者不需要发布凭据。
Dependabot PR 同样需要维护者补充已确认的依赖升级 Issue 与设计影响声明。

CI 名称 `CI`，最终必需检查为 `CI gate`。基础检查始终执行；
使用 actionlint v1.7.12 额外校验工作流表达式、任务依赖和 reusable workflow 结构。
启用后额外执行：

| 阶段 | 必须具备 | 执行检查 |
| --- | --- | --- |
| backend | go.mod、Go 测试 | gofmt、go vet、go test -race |
| frontend | web/package.json 和锁文件 | npm ci、typecheck、test:ci、build |
| integration | tests/integration 测试包 | PostgreSQL 服务上的真实集成测试 |
| container | Dockerfile、compose.yaml、部署 smoke 脚本 | 镜像构建、Compose 验证与 smoke |

CI gate 在任一已启用任务失败或取消时失败。变更工作流不能通过删除阶段逃避验证。
首次启用模块时同时建立 Core 依赖边界测试，落实代码规范。

## 发布

发布工作流仅响应 `v*` tag，但发布前还必须满足：

1. Tag 是 `vMAJOR.MINOR.PATCH` 或带预发布后缀的版本号。
2. 提交属于受保护 main 的历史。
3. 所有实现阶段已启用并通过同一套 CI。
4. 镜像构建和推送成功，记录 digest。

镜像目标 `ghcr.io/epoch-fresh-2026/wikit`，标签为版本号和提交 SHA；
不依赖浮动 latest。发布过程不连接生产服务器。
初期仅 Linux amd64；新增架构需增加构建与运行验收。

PR 工作流为只读 `pull_request`，不用 `pull_request_target` 执行贡献者代码。
发布 job 才有 `packages: write`，使用 GITHUB_TOKEN，不保存长期 PAT。
第三方 Action 固定完整 SHA，由 Dependabot 提交更新 PR。

## 远端规则

- main 必须经 PR，要求 `CI gate`、最新基线检查、解决评审线程，
  禁止删除和强制推送。默认无 bypass；管理员必要时通过修改规则恢复并留痕。
- `v*` tag 仅仓库管理员可创建；修改和删除被独立规则禁止，
  避免“可发布”同时意味着“可重写已发布版本”。
- 开启 Issues，关闭合并提交与 rebase，仅保留 squash merge，并自动删除已合并分支。
- 分支规则的 JSON 配置保存在 `.github/rulesets/`，变更需评审。

当前没有要求第二位 GitHub reviewer，避免单维护者项目无法合并；不降低必需检查。
日后团队扩大后可在 Issue 中引入 CODEOWNERS 和独立批准要求。

参考：[GitHub 容器发布](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)、
[PR 安全](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target)。
