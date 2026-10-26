# 数据与事务设计

Page ID 使用 PostgreSQL 正整数 bigint 序列，预留 1，后续从 2 分配，可有间隙且不复用。
Account ID 同样为正整数 bigint，从 1 起；REST/MCP 使用规范十进制字符串，范围 1–9223372036854775807；其他对象继续 UUID。
UTC timestamptz 对外输出 RFC 3339。页面关联、ACL、媒体引用、搜索与游标都使用数字 Page ID。
审计 target_id 使用 text，兼容 Page 数字 ID 和其他 UUID。

users 及账户外键为 bigint，page_grants.subject_id 为 text，按 user 数字字符串或 group UUID 校验。
旧 UUID 账户数据库不做原地转换，校验和不匹配时拒绝执行，需显式使用新空库。

## 实体与发布约束

| 实体 | 核心字段与约束 |
| --- | --- |
| users | bigint 序列 ID、唯一 username_key、password_hash、role、status、auth_version、create_page |
| user_tokens / sessions | 摘要、代次、有效期；不保存明文凭证 |
| groups / group_members | UUID、version、create_page；成员组合唯一 |
| page_policies / page_grants | bigint page_id、visibility、published/hidden、policy_version；能力 read/propose/discuss/merge/manage |
| pages | bigint id、不可变 document/media 类型、head_revision_id |
| revisions | UUID、page_id、parent_id、source_kind、source_proposal_id、snapshot、author_id、publisher_id、created_at |
| proposals | UUID、page_id、author_id、open/closed/merged、current_revision_id、version |
| proposal_revisions | UUID、proposal_id、page_id、base_revision_id、不可变 snapshot |
| page_issues / comments / issue_proposals | 页面归属、作者、正文、状态、治理版本，同 Page 复合外键 |
| blobs / uploads | UUID、原件摘要、大小、状态、上传者、预约与租约 |
| tags / revision_tags / revision_media_refs | 规范化标签与固定发布媒体关联，随快照密封 |
| audit_events / moderation_events | 追加审计、理由、主体、请求 ID |
| outbox_events / search_state | 顺序、任务 UID、目标索引、重试状态、active_index |
| idempotency_records / list_cursors | 绑定当前账户、操作/查询与有效期 |

list_cursors 使用可空 bigint page_anchor，页面目录保存最后消费
候选的数值 ID，外部仍为随机不透明 token。其他列表继续使用 position。

source_kind 区分 bootstrap/create/merge。只有 merge 有 source_proposal_id 且要求 parent；
bootstrap 只允许 Page 1、无用户作者和发布者；create/merge 必须有作者与发布者。
记录提案作者和实际发布者是两件事，首页插入发布版本必须由当前活跃管理员发布。
父版本唯一（包括 null），来源提案唯一；复合外键确保 parent/base/head/候选归属。
快照及标签/媒体关联密封后禁止修改/删除/清空；head 只能前进到密封的直接子版本。
没有 reviews 表、审核轮次或审核写入。页面与策略禁止删除首页，首页策略 CHECK 固定公开已发布。

0008 在迁移事务中创建标题“首页”、空正文、空标签的 Page 1、系统首版、head、审计及 outbox。
迁移历史确保重复初始化不再发布也不覆盖首页。普通创建事务一次分配 ID 并完成首版发布，
失败全部回滚（序列值允许跳过），不生成占位提案。

## 授权与事务

创建先检查当前 create_page；合并先检查当前 merge。所有业务使用统一授权 advisory lock，
再锁 Page、Proposal，检查预期 head/候选、媒体 ACL 和 ready blob。
Revision、关联、head、提案状态（仅合并）、审计、outbox 和幂等结果一次提交。
账户/组/页面治理与发布使用同一锁确定提交顺序，不依赖事务外权限缓存。
治理只改变策略与状态，不改变历史；恢复页面回到 published。

outbox worker 按 Page 顺序投递并持久化远端 task UID。每次读取当前 head/ACL，不将旧事件
正文投回索引。完成远端任务后确认事件；失败保留可重试状态。搜索最终响应由 Core 再次鉴权。

## 媒体存储

媒体状态、上传提交顺序、引用保护和 GC 统一见[业务规则](../business-rules.md#媒体生命周期)。
数据库以 blob 状态和引用约束阻止无效发布；原件目录与数据库必须配套恢复。

## 迁移与持久化边界

使用编号 SQL migrations；单独迁移命令负责锁与版本，不在多个实例启动时竞跑。
schema 兼容性列入发行说明。生产不自动运行破坏性 down migration。
数据库、原件是备份对象；搜索索引及派生文件是可重建数据。

首个迁移只建立 `wikit` schema，不提前创建业务表。
内嵌 `internal/storage/postgres/migrations/NNNN_name.sql` 从 0001 连续编号；
`public.wikit_schema_migrations` 记录编号、文件名、SHA-256 和 UTC 应用时间。
独立 migrate 命令在一个事务内获取固定 transaction advisory lock，然后校验历史，
应用全部待执行 SQL 并记录版本；失败回滚整批，取消／超时释放锁。
当前使用不兼容的新部署基线，旧库校验和不匹配即拒绝，不清空、不原地转换；此基线发布后已应用迁移不得修改。历史缺口、校验和差异、未知新版本均拒绝执行。
应用启动与 readiness 均要求历史与本二进制内嵌迁移完全一致，不隐式升级数据库。
SQL 文件通过 `.gitattributes` 固定 LF，避免跨平台 checkout 改变校验和。
迁移 SQL 禁止自行控制事务或包含不能在事务内执行的语句；无 down 命令。
