# Wikit

[English](README.md) · [设计文档](docs/README.md) · [贡献指南](CONTRIBUTING.md)

Wikit 是面向公开社区的开源、自托管 Wiki。它借鉴 Git 的提案、diff 审核、
合并与版本历史；文档和媒体都是独立 Page，以标签组织。人和 Agent 通过
REST 与 MCP 使用同一套账户权限。

**当前状态：已实现账户/授权治理、不可变历史、审核与原子发布、三方 rebase、撤销/恢复提案和 PostgreSQL 测试；
媒体上传、鉴权 Range 下载、本地安全 GC、页面议题与治理、中英文搜索及索引恢复已实现；官方 SDK 提供 41 个 MCP 工具，复用同一 Core。部署仍待实现，尚无正式镜像。**
后续工作见[开发路线](docs/roadmap.md)与
[GitHub Issues](https://github.com/epoch-fresh-2026/Wikit/issues)。

## 架构

React / TypeScript / Vite / shadcn/ui 通过 REST 访问 Go Core；
Agent 通过 MCP Streamable HTTP 访问同一个 Core。
PostgreSQL 保存权威业务状态，本地持久卷保存不可变媒体，
Meilisearch 提供可重建的中英文检索。运行时不依赖 Git。

v0.1 面向单机 Docker Compose，设计覆盖公开与受限页面、页面权限、
单页面提案、固定版本媒体引用、审核、恢复与撤销提案、页面 Issue。
原生 Git 接口、多租户、模板、实时协作、通知、语义搜索、S3 和高可用不在首版范围。

## 验证文档与契约

安装 Node.js 24 后运行：

```sh
npm ci
npm run check
```

这些命令检查文档、REST/MCP 契约及仓库工作流，不代表应用功能已经通过测试。
实现阶段逐步启用不可跳过的应用检查。参阅[开发说明](docs/development.md)、
[部署恢复设计](docs/operations.md)。

## 参与开发

先在 GitHub Issue 中确认设计与验收条件，再更新文档、实现代码并提交关联 PR。
必须遵循**高内聚、低耦合、模块化、可扩展**的[代码规范](docs/coding-standards.md)。
详细设计使用中文，代码与 API 标识符使用英文。

## 协议

项目源码采用 [MIT](LICENSE)。站点用户内容由站点单独声明许可，不自动套用源码协议。
