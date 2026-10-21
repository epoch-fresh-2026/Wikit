# Issue #35 验证记录

本次是新部署基线，旧 UUID 数据库会被迁移历史校验拒绝，不做自动转换或删除。

2026-10-21 本地实际运行：

- `npm run check`：通过（仓库、OpenAPI 与 9 项脚本测试）。
- `npm run typecheck --prefix web`、`npm run test:ci --prefix web`、`npm run build --prefix web`：通过，32 项前端测试。
- `go vet ./...`、`go test -race -count=1 ./...`：通过。
- PostgreSQL 18 / Meilisearch 1.55：`go test -race -timeout 300s -tags=integration ./tests/integration/... -count=1` 通过（123.961s）。
- 额外的超过 JS 安全整数范围 Page ID、创建与撤权提交顺序测试启用 race 通过（7.494s）。
- 测试数据库由现有集成夹具逐例创建和清理；没有迁移或删除现有站点数据库。

2026-10-21 远端完整 [CI 39600938541](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39600938541) 通过，对应实现提交 `6dafc552fe6e2fac4b6d14142e97d66333a05235`。Documents and contracts、Go、WebUI、PostgreSQL integration、Container and Compose、CI gate 均成功，所有实现检查阶段保持启用。后续提交仅补充说明与此验收记录；最终 PR 检查仍需通过。

[容器日志](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39600938541/job/119226856842) 实际验证：

- 创建账户、授予创建权、直接发布、修改提案与版主直接合并、历史及搜索更新。
- 新库备份恢复、媒体 SHA-256 与大小核验、审计、搜索重建；损坏备份与非空恢复目标均被拒绝。
- 旧基线 `f3e284fbf845d46903da0f33c29938a57ff2b98f` 使用自身应用及验收程序生成旧备份；新镜像明确拒绝旧 UUID schema，旧镜像恢复旧备份后通过验收。
- 100,000 条离线页面夹具（80% 公开、10% 受限、10% 隐藏，正文约 1 KiB），完整索引重建 124 秒。
- 数据集达到 100,004 页时，新增发布到搜索可见为 417.648 ms（100 ms 轮询）。
- 100 并发，每阶段 2,000 次真实 HTTP 搜索；首次与预热阶段均 0 错误、0 越权结果。

| 阶段 | P50 ms | P95 ms | P99 ms | 请求/秒 |
| --- | ---: | ---: | ---: | ---: |
| 首次 | 4879.571 | 5218.529 | 5370.548 | 20.35 |
| 预热 | 4886.459 | 5219.207 | 5394.027 | 20.33 |

这些是共享 CI runner 的实际观测，不是生产性能承诺。规模检查验证请求成功和权限隔离，未设置延迟 SLO。本地无 Docker，以上容器证据来自 CI。

本机独立数据库预览启动命令被自动审批拒绝（仅返回 blocked by policy），没有执行；不将组件测试称作浏览器端到端或容器验收。

浏览器排版检查使用仅驻留内存的示例 API：已实际检查 Wiki 阅读（三栏）、后台账户/用户组独立布局、Enter 标签成项及离开编辑 AlertDialog；这是前端交互检查，不替代真实后端闭环。

补充回归：URL 筛选变化保留输入焦点，讨论筛选与历史比较条件可随路由恢复。前端 32 项测试通过。
