# 页面快照与固定媒体引用

状态：accepted；日期：2026-10-20；对应已确认的 Issue #4。

页面元数据与内容版本由 pages 模块拥有；候选类型由 proposals 模块公开，通用的
document/media 快照值由 content 模块拥有，避免 Page 与 Proposal 相互循环导入。
页面创建用例在同一授权事务写入 Page、初始 ACL、Proposal、不可变候选及审计/幂等结果。

Unicode NFC/case-fold 和 CommonMark AST 解析由适配器提供原语；Core 定义标题、标签、
URL 白名单和媒体引用规则。x/text v0.38.0、Goldmark v1.8.6 锁在 go.mod/go.sum。
使用 AST 避免把代码示例或未使用的 Markdown 链接定义误当成存活媒体引用。

复合外键与不可变触发器保护内容、候选、标签和媒体关联；治理属性留在 page_policies。
快照先在事务内插入关联，再 seal；延迟约束禁止未 seal 快照提交，seal 后禁止补写关联。
blob 元数据先落地以验证 media 快照引用，真实上传/文件操作留给 #6。
没有公开直接发布接口；#5 必须复用授权事务、版本关系与不可变约束实现审核合并。
