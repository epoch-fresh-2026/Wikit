# 代码规范

高内聚、低耦合、模块化和可扩展是必须满足的评审要求，不只是目录命名建议。

## 高内聚

- 按业务能力划分账户、权限、页面、提案、讨论、媒体、检索模块。
  一个用例的校验、权限要求、状态转换和错误定义应在同一业务边界内理解。
- REST、MCP、worker、前端不各自实现审核或权限规则。
- 事务边界由业务用例定义，不能拆成多个各自提交的 repository 调用。
- 不创建吸收各种无关代码的 `utils`、`common`、`manager` 万能模块。

## 低耦合与模块边界

建议结构（实现阶段创建实际需要的目录）：

```text
cmd/wikit/                 composition root、启动与命令
internal/core/             用例协调、公共业务 ID 与错误、模块接口
internal/core/identity/
internal/core/access/
internal/core/pages/
internal/core/proposals/
internal/core/discussions/
internal/core/media/
internal/core/search/
internal/adapters/http/    REST 与会话边界
internal/adapters/mcp/     MCP 工具与错误映射
internal/storage/postgres/
internal/storage/blobfs/
internal/search/meili/
internal/worker/
web/src/features/         按页面阅读、提案、账户等能力组织
web/src/components/ui/    shadcn/ui 基础组件
```

- Core 不依赖 HTTP、MCP SDK、SQL driver、Meilisearch client 或文件系统实现。
- 适配器依赖 Core；具体实现仅在 composition root 装配。
- 模块只能依赖其他模块显式公开的接口与业务类型，不读取其内部表或私有状态。
  跨模块事务通过显式 Unit of Work 协调，不导出裸 SQL transaction 到传输层。
- 接口由使用方定义，保持用例所需的最小能力；禁止为了 mock 为每个 struct
  添加镜像接口。共享类型必须有清晰归属，不用通用 `map[string]any` 传业务对象。
- 禁止循环依赖、可变全局服务定位器和跨请求共享可变认证状态。

## 可扩展

- 文件存储与搜索通过 `BlobStore`、`SearchIndex` 等小接口替换，
  接口约定必须覆盖失败、取消、一致性和重试，而非只抽象方法名。
- 保持一个真实实现即可；首版不实现 S3、插件注册系统、事件总线框架或通用 ORM
  仓储来证明“可扩展”。新实现必须由明确 Issue 驱动。
- API 传输 DTO 与持久化结构分离；版本与权限字段必须显式映射。
- 数据迁移可追踪；兼容性变化同步修订契约、ADR、消费者及测试。

## Go

- 使用 gofmt、go vet；传入 `context.Context`，传播取消和超时。
- 显式返回并包装错误；Core 使用稳定错误类别，适配器映射协议错误。
- 禁止忽略错误、业务路径 panic、将秘密或内容写入错误日志。
- 构造函数注入依赖；时钟、ID 生成等仅在确定性测试需要时抽象。
- 并发更新必须依赖数据库事务和版本条件，进程内锁不能作为唯一一致性保证。

## React / TypeScript

- TypeScript strict 模式，禁止无解释的 `any` 和跳过检查。
- 功能模块拥有自己的数据访问、状态和视图；基础 UI 不依赖具体业务请求。
- API client 集中处理认证、错误和契约类型；React 组件不自行拼装权限算法。
- 服务端数据与短期编辑状态分开；提交前保留基础版本，冲突不得静默覆盖。
- 提供键盘操作、标签、焦点与错误反馈；隐藏按钮不等于服务端授权。

## PR 必答问题

1. 业务规则是否集中，REST/MCP 是否只调用同一个用例？
2. 依赖方向是否正确，是否跨越了模块私有实现？
3. 新接口是否对应真实替换点，是否引入不必要的通用层？
4. 事务、权限与失败处理是否完整，契约和文档是否已更新？
5. 测试是否验证外部行为与边界，而不是重复实现细节？

自动化负责格式、类型、测试和依赖边界检查；高内聚与合理抽象仍必须人工评审。
后端模块首次落地时添加依赖边界测试，防止 Core 引入适配器或存储实现。

Issue #2 首次落地 `internal/core/health`，由该消费方定义 `Check(context.Context) error`
小接口；HTTP 仅将健康用例结果映射为 200/503，具体 PostgreSQL／目录实现由
`cmd/wikit` 注入。Core 边界测试遍历包的直接与传递依赖，禁止第三方库、协议、
数据库及文件系统实现进入 Core；后续业务模块按实际用例建立，不创建空模块。
