# 数据与事务设计

本文件是逻辑模型，不是已经部署的 SQL migration。
ID 统一为 UUID，时间为 UTC timestamptz；API 输出 RFC 3339。

## 实体

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
