# 数据与事务设计

> Issue #35 当前设计以 [ADR 0018](adr/0018-wiki-publication-redesign.md) 为准：数字 Page ID、固定首页、独立创建权限、直接首版发布、版主直接合并和 Wiki SPA。下文旧 Issue 的实现记录不作为新基线验收证据。

下列业务实体仍是逻辑模型，后续 Issue 逐步提供 SQL migration。
ID 统一为 UUID，时间为 UTC timestamptz；API 输出 RFC 3339。

## 实体

Issue #8 的 0007 migration 为 outbox 增加顺序、持久 task UID、目标索引及安全错误代码，
新增 search_state 保存 active_index。政策变更触发器与业务事务一起追加事件；事件的
revision_id 可空以表示权限/隐藏/修复。worker 每次读取当前 Page/head/ACL 投影，不使用旧事件正文。

Issue #7 的 0006 migration 增加 page_issues、comments、issue_proposals 与 moderation_events。
关联用两侧 (对象 ID, page_id) 复合外键强制同 Page，唯一关联不可更改或删除。
Issue 标题/正文、评论正文及归属创建后不可变，状态/隐藏修改递增对象版本；无删除入口。
moderation_events 追加保存理由、目标、操作账户与请求 ID，和状态、普通审计、幂等结果同事务提交。
Page moderation 仅改变策略 status/version；恢复按当前 head 还原 published/proposed，不改写 ACL 或历史。

Issue #5 的 0004 migration 增加不可变 reviews（递增 sequence、明确候选/base/round）与
outbox_events。审核候选与提案使用复合外键；合并写入发布快照、关联、head、提案状态、
审计、outbox 与幂等结果，一次提交。沿用授权事务锁，再依次行锁 Page、Proposal。
此阶段只持久化待投递事件，搜索投递 worker 仍属于 #8。

Issue #4 的 0003 migration 增加 pages、revisions、proposals、proposal_revisions、
blobs 元数据、tags、两类快照标签关联及两类快照媒体引用。blobs 只有元数据模型与
ready 检查，上传/原件存储实现仍属于 #6。页面类型创建后不可变；治理属性继续由
page_policies 独占，API 显式组合，内容恢复不修改策略。

复合外键约束 parent/base/head 的 Page 归属和 current candidate 的 Proposal 归属。
发布来源 Proposal 唯一，父版本唯一（含 null）；写入发布版本时锁页面并校验旧 head，
head 只可前进到其直接子版本。两个快照表及其标签/媒体关联禁止 UPDATE/DELETE/TRUNCATE。
快照与关联在同一事务插入；媒体目标必须是已发布 media Revision，media 快照引用 ready blob。
此阶段通过真实数据库约束测试验证这些性质，不提供绕过审核的发布命令。

Issue #3 的 0002 migration 实现 users、user_tokens（含撤销后的代次记录）、sessions、
groups/group_members、page_policies/page_grants、audit_events、auth_limits、
idempotency_records 和 list_cursors。page_policies 持有页面 ACL 与状态，后续 pages
通过同一 UUID 关联，避免安全模块读取内容模块私有表。业务 Page 尚未公开创建。
账户 auth_version、组 version、策略 policy_version 防止过期覆盖；安全变更与审计原子提交。

| 实体 | 核心字段与约束 |
| --- | --- |
| users | id、username_key unique、password_hash、role、status、auth_version |
| user_tokens | user_id unique、digest unique、generation、created_at；零或一个有效记录 |
| sessions | id_digest、user_id、csrf_digest、expires_at、auth_version |
| groups / group_members | group ID；成员 `(group_id,user_id)` unique |
| pages | id、type、visibility、status、head_revision_id nullable、policy_version |
| page_grants | page_id、subject_type、subject_id、capability；组合 unique |
| revisions | id、page_id、parent_id、source_proposal_id、snapshot、author、created_at |
| proposals | id、page_id、author、state、current_revision_id、review_round、version |
| proposal_revisions | id、proposal_id、base_revision_id nullable、snapshot、created_at |
| reviews | id、proposal_id、proposal_revision_id、base_revision_id、round、actor、decision |
| page_issues / comments | page_id、author、state／hidden、version、正文 |
| issue_proposals | issue_id、proposal_id；必须属于同一 Page |
| blobs | id、digest、size、mime、storage_key、state、uploader、created_at |
| revision_media_refs | 来源发布／提案快照、目标 media Page 与发布 revision |
| tags / revision_tags | 规范化 tag key unique；关联不可变快照 |
| audit_events | actor、credential_generation、action、target、request_id、时间、变更摘要 |
| outbox_events | event ID、page_id、kind、attempts、next_attempt、lease、完成状态 |
| idempotency_records | actor、operation、key unique、request_hash、result、expires_at |

snapshot 区分 document 与 media。document 为 title、tags、markdown；
media 为 title、tags、description、blob_id。发布内容不可 UPDATE/DELETE；
通过数据库权限及约束测试防止应用误改。治理元数据在 pages 上，不能混入内容回滚。

外键必须保证 parent/base/head 属于正确 Page，candidate 属于正确 Proposal。
合并来源 proposal 唯一；一个 Proposal 最多对应一次发布。
引用目标必须为已发布媒体 Revision，不能指向提案快照。
父版本不可成环；发布 parent 只能是事务内锁定的旧 head。

## 合并事务

1. 获取账户及涉及授权记录的一致视图；锁 Page，再锁 Proposal。
2. 校验请求预期版本、状态、当前权限、有效审核、媒体引用和可用 blob。
3. 插入不可变 Revision，parent = 旧 head。
4. 更新 Page.head、Proposal.state、审计、outbox 与幂等结果。
5. 一次提交。任何失败均不产生可见的部分发布。

账户禁用、角色、组成员和 ACL 变化需要通过统一授权协调锁／代次重检与上述
事务序列化。锁顺序为账户与组（按 ID 排序）、Page（按 ID 排序）、Proposal。
实现阶段用集成测试证明权限收回和合并竞态，而非仅检查代码顺序。

outbox 与业务变更同事务写入。worker 使用数据库租约领取工作；
对同 Page 串行投递，等待搜索 task 完成后再确认事件。
重试读取当前 Page，不将过期事件携带的旧内容重新写回搜索。

## 媒体原件

Issue #6 增加 uploads 预约（1 小时）、有界写入租约和 blob GC 墓碑状态。
上传先以短授权事务领取唯一 attempt，流式写 staging 文件不持有数据库连接或授权锁；
完成时重新鉴权并检查预约/租约，在同一授权事务内提交原件、登记 ready blob、更新预约和审计。
100 MiB 为默认上限，实际字节必须与预约大小完全一致，声明 MIME 与文件头识别结果一致。
PUT 按预约身份幂等：ready 后同字节返回原 blob，不同字节拒绝，失败可在预约有效期内重试。
原件以随机 UUID 存储，不做跨账户去重；响应不包含物理路径。

GC 先在授权事务内把超过宽限期且不被任何候选/发布历史引用的 blob 标记 deleting，
提交后删文件，再标记 deleted；失败保留可重试墓碑，不重新变为 ready。
发布与 GC 共享授权事务锁，deleting 永远不能发布；全部候选历史（包括关闭提案）永久保护原件。
孤立 objects/staging 文件也按宽限期清理，每个删除前在同一锁内重查 blob 与活跃上传租约。
文件删除失败不撤回删除意图；数据库登记失败留下孤立文件，延后清理。

上传授权与读取授权分离。未关联上传只允许上传者及管理员读取；
不存在公开的按 hash 下载接口。关联到候选或发布版本后，下载通过该 Page
对应的可访问候选／版本端点鉴权，不能借相同 blob 绕过其他 Page ACL。

写入流程：限流及限额 → 流式写临时文件并计算 SHA-256 → 检查 MIME 与文件头
及大小 → fsync 原件 → 同卷原子 rename 至不可变路径 → fsync 目录 →
登记 ready blob。数据库发布只能引用 ready blob。
磁盘写入失败不进入 ready；登记失败留下孤立文件，之后由 GC 回收。
上传响应不暴露服务器路径或去重命中信息。

首版接受 JPEG/PNG/GIF/WebP、MP3/WAV/OGG、MP4/WebM、PDF。
不接受 SVG、HTML、脚本或可执行文件。浏览器无法播放的合法原件提供下载，
不承诺服务端转码。内容下载设置正确 MIME、nosniff，PDF 预览使用受限容器。

GC 默认宽限期 7 天。提案的全部历史快照、发布历史、媒体引用和进行中上传
都算存活引用；关闭提案不会立刻释放其历史。
上传与 GC 通过状态转换和锁协调，不得删除准备发布的 blob。
派生预览可重建；原件不可通过重建搜索恢复。

## 迁移与持久化边界

使用编号 SQL migrations；单独迁移命令负责锁与版本，不在多个实例启动时竞跑。
schema 兼容性列入发行说明。生产不自动运行破坏性 down migration。
数据库、原件是备份对象；搜索索引及派生文件是可重建数据。

Issue #2 的首个迁移只建立 `wikit` schema，不提前创建业务表。
内嵌 `internal/storage/postgres/migrations/NNNN_name.sql` 从 0001 连续编号；
`public.wikit_schema_migrations` 记录编号、文件名、SHA-256 和 UTC 应用时间。
独立 migrate 命令在一个事务内获取固定 transaction advisory lock，然后校验历史，
应用全部待执行 SQL 并记录版本；失败回滚整批，取消／超时释放锁。
已应用迁移不得修改；历史缺口、校验和差异、未知新版本均拒绝执行。
应用启动与 readiness 均要求历史与本二进制内嵌迁移完全一致，不隐式升级数据库。
SQL 文件通过 `.gitattributes` 固定 LF，避免跨平台 checkout 改变校验和。
迁移 SQL 禁止自行控制事务或包含不能在事务内执行的语句；无 down 命令。
