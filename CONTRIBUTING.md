# 贡献指南

详细设计和贡献讨论使用中文；英文贡献可先在 Issue 中说明。

遵循 **需求分析 → Issue → 设计 → 更新文档 → 开发 → PR → 审查 → 合并**。
完整工作流与文档维护规则以 [AGENTS.md](AGENTS.md) 为准。

先分析需求与现有实现，再创建或领取 GitHub Issue，记录问题、范围、依赖与可检查的验收条件，链接[文档导航](docs/README.md)中相关章节。
先统一修订需求、业务规则、模块设计、接口和测试要求，由维护者确认并添加 `design-approved`，再实现。
无需单独的设计 PR；规格正文不登记开发 Issue，进度和证据由 Issue 与 PR 承载。

分支使用 `docs/<issue>-<topic>`、`feat/<issue>-<topic>` 或 `fix/<issue>-<topic>`。
PR 按模板关联 Issue、声明设计影响，并如实填写验证结果。部分交付使用 `Refs #N`，完整交付使用 `Closes #N`。
默认持续完成 PR 审查、问题修复和全部已启用检查，再按分支保护规则合并并确认 Issue 关闭；不在本地修改或提 PR 后自行结束。
自动检查不替代人工评审；不强制第二名 GitHub reviewer，产品内的提案规则另由业务文档定义。

阅读[代码规范](docs/engineering/coding-standards.md)，重点检查业务规则归 Core、模块低耦合、接口小而有实际用途，禁止跨模块访问私有实现。
所有仓库变更通过 PR 交付；不要提交真实凭证、用户内容、数据库备份或本地 `.env`。
