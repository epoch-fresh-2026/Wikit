# 贡献指南

详细设计和贡献讨论使用中文；英文贡献可先在 Issue 中说明。

1. 创建或领取 [GitHub Issue](https://github.com/epoch-fresh-2026/Wikit/issues)，
   明确问题、范围、设计影响、依赖及验收条件。
2. 维护者确认方案后添加 `design-approved` 标签。无需独立设计 PR。
3. 分支命名为 `docs/<issue>-<topic>`、`feat/<issue>-<topic>` 或
   `fix/<issue>-<topic>`。先提交文档，再提交实现。
4. PR 使用模板，填写 `Closes #123` 和 `Design-Impact: updated`；
   无设计影响写 `Design-Impact: none` 和具体 `Design-Reason:`。
5. 运行 `npm run check` 和已启用的应用检查，提交结果。
6. 通过检查、解决评审意见后由有写权限的维护者合并。

必须阅读[代码规范](docs/coding-standards.md)。特别检查高内聚、低耦合、
模块化与可扩展：规则归 Core、接口小而有实际用途、禁止跨模块访问私有实现。

自动检查只验证 Issue 存在、方案已确认、文档声明及文件变化，不替代维护者判断。
仓库不强制第二名 GitHub reviewer；应用内的提案审核规则与 GitHub
仓库规则分别定义，应用始终遵守协作状态机。

所有变更通过 PR 提交。
不要提交真实凭证、用户内容、数据库备份或本地 `.env`。
