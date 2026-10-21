# 设计文档索引

状态：2026-10-20 确认的 v0.1 基线；描述目标行为，不表示应用已实现。

| 文档 | 内容 |
| --- | --- |
| [领域术语](../CONTEXT.md) | Page、版本、提案、审核、撤销与恢复 |
| [产品与架构](design.md) | 范围、组件、数据流和页面体验 |
| [协作状态机](collaboration.md) | 发布、审核、三方合并与回滚 |
| [权限与账户](permissions.md) | 权限矩阵、Token、会话、治理 |
| [数据设计](data-model.md) | 实体、约束、事务及媒体生命周期 |
| [检索](search.md) | 中文检索、索引一致性、权限过滤 |
| [REST 契约](api.md) | HTTP 约定及机器可读 OpenAPI |
| [MCP 契约](mcp.md) | 工具目录、认证和错误语义 |
| [WebUI](web.md) | 页面、编辑审核、状态隔离与前端验证 |
| [部署恢复](operations.md) | 配置、备份、升级及恢复 |
| [代码规范](coding-standards.md) | 高内聚、低耦合、模块化、可扩展 |
| [开发工作流](development.md) | Issue、文档、PR、CI/CD |
| [验收计划](testing.md) | 必测场景与验收证据 |
| [发布候选验收](release-readiness.md) | 容器、恢复、规模测试及受控发布状态 |
| [开发路线](roadmap.md) | 阶段依赖与 GitHub Issue |
| [ADR](adr/README.md) | 关键决策与取舍 |

规格优先级：已接受 ADR 与设计约束 → REST/MCP 机器契约 → 实现。
出现矛盾时先在 Issue 中解决并同步修订文档，不通过代码悄然改变约定。
