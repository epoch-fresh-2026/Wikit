# 文档导航与维护规则

文档按职责组织，描述完整的现行约定。设计不是实现证明，运行结果以对应提交和产物的实际证据为准。

| 分类 | 文档 | 唯一职责 |
| --- | --- | --- |
| 领域语言 | [术语](../CONTEXT.md) | 核心概念与命名 |
| 产品 | [需求](requirements.md) | 用户目标、功能范围、非功能要求与排除项 |
| 业务 | [业务规则](business-rules.md) | 内容、身份权限、协作状态机、讨论、媒体与检索规则 |
| 架构 | [系统架构](architecture/overview.md) | 组件、部署形态与总体数据流 |
| 架构 | [模块划分与协作](architecture/modules.md) | 模块职责、依赖接口、调用链与事务交接 |
| 架构 | [数据设计](architecture/data-model.md) | 表结构、约束、持久化和迁移 |
| 架构 | [架构决策](architecture/decisions.md) | 当前选择的理由、替代方案与代价 |
| 接口 | [REST](interfaces/rest.md)、[MCP](interfaces/mcp.md)、[WebUI](interfaces/web.md) | 传输字段、协议映射、路由及交互 |
| 工程 | [开发](engineering/development.md)、[代码规范](engineering/coding-standards.md) | 工作流执行、构建、检查及实现约束 |
| 工程 | [依赖维护](engineering/dependencies.md)、[部署恢复](engineering/operations.md) | 工具链边界、依赖升级与运行维护 |
| 质量 | [测试策略](quality/testing.md)、[性能验证](quality/performance.md) | 验收场景、验证方法、测量口径 |
| 质量 | [发布验收](quality/release-readiness.md) | 发布门槛、证据要求与产物确认 |

## 阅读与变更路径

先读术语、需求和业务规则，再按影响范围阅读模块协作、数据设计及接口；实现与验收遵循工程和质量文档。
需求说明“需要什么”，业务规则说明“如何判定”，架构说明“由谁完成、如何协作”，接口说明“如何调用或展示”。
同一规则只在所属文档完整定义，其他文档引用它，不复制另一套规则。
机器契约与文字契约必须一致；发生冲突时先确认意图、同步修订，不能选择较宽松的描述绕过约束。

## 统一维护

直接重写受影响章节，使整份文档自洽；同时检查术语、上下游接口、验收与导航。
不以“补充”“某次改版”“某日期更新”追加例外，不按文档版本另建平行规格。
重要取舍修订架构决策的相应主题，历史内容通过 Git 查询。
依赖版本以清单与锁文件为准；业务 Revision、协议路径和迁移编号是技术标识，不是文档版本。

开发流程统一为 **Issue → Design → Implementation → PR → Review → Merge**，具体要求见[AGENTS.md](../AGENTS.md)。
Issue 链接相关文档章节、承载方案讨论和验收进度；规格正文不写开发 Issue 编号或链接。
运行证据放在 PR、CI 日志及产物中，长期文档保留验证方法、判定标准和必要的兼容性边界。
