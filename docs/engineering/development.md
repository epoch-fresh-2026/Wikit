# 开发工作流与 CI/CD

## 工程结构与运行基线

Go composition root 位于 cmd/wikit；Core 承载领域规则，适配器处理 HTTP、MCP、文本和外部存储。
WebUI 位于 web，机器契约由 scripts/build-contracts.mjs 生成，正式测试位于各模块及 tests/integration。
实验题集、冻结包和运行报告放在被 Git 忽略的 .local，不与正式夹具混放。
仓库文档检查包含未跟踪的新文件，但不进入含自身 .git 的独立仓库或 worktree；这些工程独立验证。

依赖与运行环境以清单、锁文件和构建配置为准，维护规则见[依赖维护](dependencies.md)。
根目录提供契约生成工具，Web 使用独立工具链，两处依赖均需安装。
真实集成测试连接专用 PostgreSQL 与 Meilisearch；MCP 使用官方 SDK 客户端验证协议映射。
测试覆盖范围和检查入口统一见[测试策略](../quality/testing.md)。

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
当前所有阶段均启用，对应路径出现时检查会要求启用阶段；
阶段一旦启用，后续 PR 不允许回退。新增阶段必须带真实测试，不能靠空测试通过。
不能把跳过的应用测试宣称为已验证。
`release_ready` 是额外发布门槛，只有完成全部产品验收并提交发布证据后才能启用。

## 工作流

执行 **Issue → Design → Implementation → PR → Review → Merge**，阶段要求以[AGENTS.md](../../AGENTS.md)为准。
Issue 链接需求、业务规则、模块协作、接口及验收章节；规格中不反向记录开发 Issue。
设计经维护者确认并标记 `design-approved` 后实施，普通修复与依赖升级同样遵守该流程。
PR 使用模板声明设计影响与实际检查结果，完整交付关闭 Issue，部分交付保留未完成验收项。
外部贡献者不需要发布凭据。

CI 名称 `CI`，最终必需检查为 `CI gate`。基础检查始终执行；
使用工作流锁定的 actionlint 校验工作流表达式、任务依赖和 reusable workflow 结构。
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
