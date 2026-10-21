# 协作状态机

> Issue #35 当前设计以 [ADR 0018](adr/0018-wiki-publication-redesign.md) 为准：数字 Page ID、固定首页、独立创建权限、直接首版发布、版主直接合并和 Wiki SPA。下文旧 Issue 的实现记录不作为新基线验收证据。

## 提案与审核

Page 发布历史为线性序列。Proposal 保存独立的不可变候选快照，
每个快照包含 `base_revision_id`，首次创建页面时为 null。
提案版本不能作为已发布内容被普通页面／媒体读取接口返回。

| 操作 | 前置条件 | 结果 |
| --- | --- | --- |
| 创建 | 可读且有 propose；新建时为活跃账户 | open 提案与第一个快照 |
| 更新 | open，作者或页面管理者，仍有 propose | 新快照，旧审核失效 |
| 审核 | open，可读且有 review | 绑定当前候选与基础版本的审核 |
| 关闭 | open，作者或页面管理者 | closed，审核不再用于合并 |
| 重开 | closed，作者或页面管理者且有 propose | open，新审核轮次，重新审核 |
| 合并 | open，有 review，有效批准，基础仍为 head | 新发布版本，merged |

`approve`、`request_changes`、`comment` 是审核意见，均不可变。
同一账户对同一审核轮次的最新 approve/request_changes 决定该账户立场；
comment 不改变立场。至少一个当前仍有审核权限的账户批准，且没有当前仍有
审核权限账户的未解决 request_changes，才能合并。提出 request_changes 的账户
可改为批准，或提案更新使整轮审核失效。管理者不能直接删除负面审核。

作者拥有 review 时允许自审、自合并。共享同一账户 Token 的多个 Agent
仍算一个账户，不构成多个独立审核者。权限收回或账户禁用立即使其批准失去效力。

合并请求必须提供 `expected_head_revision_id`（创建时 null）与
`expected_proposal_revision_id`。合并事务依次锁定 Page、Proposal，
重新检查当前权限、版本、审核、引用媒体和 blob 状态，再原子写入发布版本、
head、merged 状态、审计、outbox。权限／账户治理事务与合并共享相关授权版本锁，
不能仅在事务外做一次权限检查。

同一提案合并请求使用同一个 Idempotency-Key 重试时返回原结果；新 key 对已经
merged 的提案返回 `invalid_state`。禁止把传输重试误当新发布。

## 三方合并

Issue #5 将状态、审核、diff、rebase、merge、revert/restore 作为同一 Core 提案服务的用例。
所有业务写入先在事务内验证当前 read，再重放幂等结果或检查操作权限；
审核记录按数据库递增序号确定同账户的最新立场，不依赖同时间戳排序。
关闭不要求仍有 propose，但必须仍可读且为作者或管理者；重开递增轮次。
更新候选和 rebase 同时递增 proposal version 与 review round；审核本身不改候选版本。
已过期 base 不接受新审核，需先 rebase 或提交明确解决后的完整候选。

逐行差异由注入的文本适配器计算，Core 组合两侧不重叠的编辑；相同重叠结果兼容，
不同重叠结果返回整个 Markdown 字段的结构化冲突。保留换行及末行无换行状态。
diff 返回 base 与候选的字段值（Markdown 同时提供 unified diff），stale 独立反映 base/head。
空 base 按同类型空快照参与首次发布竞争的三方比较。

媒体候选允许复用本 Page 的已发布或候选历史 blob（包括关闭提案），新 blob 仍要求
上传者本人或管理员；复用不授予跨 Page 或未关联 blob 的访问权限。所有发布/回滚引用
在事务内重查当前目标 ACL、固定媒体版本和 ready blob。

设 B 为候选基础发布快照，H 为当前发布快照，P 为候选快照。

1. B = H 时可正常审核合并。
2. B ≠ H 时旧批准无效，合并返回 `stale_revision`。作者调用 rebase。
3. Markdown 使用逐行三方合并；标题、媒体 blob 和媒体描述字段分别比较；
   blob 不合并字节。双方改为相同值不冲突，只有一方变化时取变化值，
   双方改为不同值时冲突。标签作为规范化集合逐项合并。
4. 无冲突时保存新候选快照，base = H，清空有效审核并展示新 diff。
5. 有冲突时返回结构化冲突路径和 B/H/P 内容，不保存含冲突标记的候选。
   编辑者提交完整解决后快照与预期 H；H 再次变化则重新处理。
6. 不提供绕过过期检查的强制合并。

所有参与 diff 的 Page 和版本必须可读。错误中不包含另一个无权读取页面的正文。

## 撤销与恢复

- **Restore**：复制选定发布快照为新的候选，base 为当前 head；
  将明确覆盖选定版本之后的内容变化，提交前展示完整 diff。
- **Revert**：计算目标版本与其前一发布版本之间的反向修改，应用到当前 head；
  等价于以目标版本为 base、当前 head 为 ours、目标父版本为 theirs 的三方合并。
  冲突时返回待解决内容，不直接发布。
- 首次发布没有父版本，不能 revert；需要隐藏页面时使用治理操作。
- 恢复／撤销均保留当时内容和媒体引用，但不恢复 ACL、账户权限、隐藏状态。
- 回滚引用的原件若缺失或当前不可读，返回明确错误，不悄然使用最新媒体。

## Page Issue

Issue 为 open/closed；评论属于其 Page。作者或页面管理者可关闭／重开；
有 discuss 的读者可评论。首版不修改既有评论正文，补充通过新评论表达。
管理者可隐藏讨论或评论，并记录理由及审计。提案关联必须指向同一个 Page。
合并不会自动关闭关联 Issue，避免把“关联”误解为“已解决”。

Issue #7 的创建 Issue 与追加评论要求当前 read + discuss；关闭或重开要求当前 read
且为 Issue 作者或页面管理者，不要求仍有 discuss。关闭后仍可追加评论，不自动重开。
提案关联要求当前 discuss，且为 Issue 作者或页面管理者；目标须可读并属于同一 Page。
新关联递增 Issue version，重复关联相同提案不新增记录或递增 version。

普通 GET/列表始终过滤隐藏 Issue/评论，连管理员也不通过普通读取看到隐藏正文。
管理者显式使用 include_hidden=true 读取治理视图；该模式检查当前 manage，
并可处理隐藏 Page 下的讨论。隐藏 Issue 遮蔽其全部评论；恢复 Issue 不会恢复单独隐藏的评论。
状态、关联、隐藏/恢复均使用明确对象版本，审核合并不会改写讨论状态。
正文以 UTF-8 文本保存，不修改原评论；后续渲染不得执行原始 HTML。
