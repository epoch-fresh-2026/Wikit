# Wikit

[English](README.md) · [设计文档](docs/README.md) · [贡献指南](CONTRIBUTING.md)

Wikit 是面向公开社区的开源、自托管 Wiki。它借鉴 Git 的提案、差异比较、
合并与版本历史；文档和媒体都是独立 Page，以标签组织。人和 Agent 通过
REST 与 MCP 使用同一套账户权限。

当前源码已实现账户与页面权限、不可变历史、直接创建首版、提案合并与三方 rebase、
撤销与恢复、媒体上传与鉴权下载、页面议题与治理、中英文搜索和索引恢复。
Wiki WebUI 与 39 个 MCP 工具复用同一 Core；MCP 还提供基于全文检索、带固定版本引用的有界 RAG 上下文。

Page 和 Account 使用数据库分配的正整数 ID，接口以精确十进制字符串传输。
首页固定，创建需要显式授权，后续修改通过提案发布。当前 schema 是新部署基线，
旧 UUID 数据库会被拒绝，不自动转换或清空。部署、兼容性、备份与回退见[部署恢复](docs/engineering/operations.md)。

现行需求与契约见[文档导航](docs/README.md)。发布必须针对具体源码提交及镜像 digest 重新验收，
历史检查不能证明当前源码可发布，要求见[发布验收](docs/quality/release-readiness.md)。生产部署由维护者手动执行。

## 架构

React / TypeScript / Vite / shadcn/ui 通过 REST 访问 Go Core；
Agent 通过 MCP Streamable HTTP 访问同一个 Core。
PostgreSQL 保存权威业务状态，本地持久卷保存不可变媒体，
Meilisearch 提供可重建的中英文检索。运行时不依赖 Git。

部署面向单机 Docker Compose，设计覆盖公开与受限页面、页面权限、
单页面提案、固定版本媒体引用、合并、恢复与撤销提案、页面 Issue。
原生 Git 接口、多租户、模板、实时协作、通知、语义搜索、S3 和高可用不在首版范围。

## 验证文档与契约

安装 Node.js 24 后运行：

```sh
npm ci
npm run check
```

这些命令检查文档、REST/MCP 契约及仓库工作流，不代表应用功能已经通过测试。
后端、前端、数据库集成与容器检查均为必需阶段，参阅[开发说明](docs/engineering/development.md)和
[测试策略](docs/quality/testing.md)。运行配置与恢复流程见[部署手册](docs/engineering/operations.md)。

WebUI 开发先运行 `npm ci --prefix web`，再运行 `npm run dev --prefix web`。
后端 origin 配置、状态边界及检查命令见 [WebUI](docs/interfaces/web.md)。

## 参与开发

先在 GitHub Issue 中确认设计与验收条件，再更新文档、实现代码并提交关联 PR。
必须遵循**高内聚、低耦合、模块化、可扩展**的[代码规范](docs/engineering/coding-standards.md)。
详细设计使用中文，代码与 API 标识符使用英文。

## 协议

项目源码采用 [MIT](LICENSE)。站点用户内容由站点单独声明许可，不自动套用源码协议。
