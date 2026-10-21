# Issue #35 验证记录

本次是新部署基线，旧 UUID 数据库会被迁移历史校验拒绝，不做自动转换或删除。

2026-10-21 本地实际运行：

- `npm run check`：通过（仓库、OpenAPI 与 9 项脚本测试）。
- `npm run typecheck --prefix web`、`npm run test:ci --prefix web`、`npm run build --prefix web`：通过，31 项前端测试。
- `go vet ./...`、`go test -race -count=1 ./...`：通过。
- PostgreSQL 18 / Meilisearch 1.55：`go test -race -timeout 300s -tags=integration ./tests/integration/... -count=1` 通过（123.961s）。
- 额外的超过 JS 安全整数范围 Page ID、创建与撤权提交顺序测试启用 race 通过（7.494s）。
- 测试数据库由现有集成夹具逐例创建和清理；没有迁移或删除现有站点数据库。

容器、备份恢复、旧镜像配套旧备份回退、100k 页面/100 并发的验收由已启用的 CI container 阶段执行；本地无 Docker，待 CI 结果补充。旧基线固定为 f3e284fbf845d46903da0f33c29938a57ff2b98f，使用其自身的应用与验收程序。

本机独立数据库预览启动命令被自动审批拒绝（仅返回 blocked by policy），没有执行；不将组件测试称作浏览器端到端或容器验收。

浏览器排版检查使用仅驻留内存的示例 API：已实际检查 Wiki 阅读（三栏）、后台账户/用户组独立布局、Enter 标签成项及离开编辑 AlertDialog；这是前端交互检查，不替代真实后端闭环。
