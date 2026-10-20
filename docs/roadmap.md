# v0.1 开发路线

状态：设计范围已确认；以下为实现任务，不表示应用功能完成。
每项工作遵守 Issue 确认 → 文档先行 → 实现 → PR → CI 的闭环。

| 顺序 | GitHub Issue | 依赖 |
| --- | --- | --- |
| design | [#1 锁定 v0.1 设计、契约、代码规范和仓库治理](https://github.com/epoch-fresh-2026/Wikit/issues/1) | — |
| foundation | [#2 建立 Go 模块化 Core、迁移与测试基础](https://github.com/epoch-fresh-2026/Wikit/issues/2) | #1 |
| identity | [#3 实现本地账户、唯一 Token、角色和页面权限](https://github.com/epoch-fresh-2026/Wikit/issues/3) | #2 |
| pages | [#4 实现 Page、不可变发布版本、标签与媒体引用模型](https://github.com/epoch-fresh-2026/Wikit/issues/4) | #3 |
| proposals | [#5 实现提案审核、并发合并、撤销与恢复](https://github.com/epoch-fresh-2026/Wikit/issues/5) | #4 |
| media | [#6 实现独立媒体上传、固定版本下载与安全 GC](https://github.com/epoch-fresh-2026/Wikit/issues/6) | #5 |
| discussions | [#7 实现页面 Issue、评论、提案关联与治理](https://github.com/epoch-fresh-2026/Wikit/issues/7) | #5 |
| search | [#8 实现中英文搜索、事务 outbox 和索引恢复](https://github.com/epoch-fresh-2026/Wikit/issues/8) | #6、#7 |
| mcp | [#9 实现 Streamable HTTP MCP 及 REST 等价契约测试](https://github.com/epoch-fresh-2026/Wikit/issues/9) | #8 |
| web | [#10 实现 React WebUI 的阅读、编辑和审核闭环](https://github.com/epoch-fresh-2026/Wikit/issues/10) | #9 |
| release | [#11 完成 Compose、恢复演练、基准测试与 v0.1 发布验收](https://github.com/epoch-fresh-2026/Wikit/issues/11) | #10 |

媒体与讨论可在共同前置完成后独立实现；此表不授权扩大 v0.1 范围。
首版不包含模板、通知、语义搜索、S3、多租户、实时协同、原生 Git 或跨页面提案。
若需改变已确认范围，先修改 Issue、设计和必要 ADR，再实施。

发布就绪必须有真实应用、测试和恢复证据；文档阶段 CI 通过不等于 v0.1 可发布。
