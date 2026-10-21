# Wikit

> Issue #35 当前设计以 [ADR 0018](docs/adr/0018-wiki-publication-redesign.md) 为准：数字 Page ID、固定首页、独立创建权限、直接首版发布、版主直接合并和 Wiki SPA。下文旧 Issue 的实现记录不作为新基线验收证据。

Wikit 通过可审核的提案维护社区知识。文档与媒体共享协作语言。

## Language

**Page（页面）**：
独立管理可见性、协作和发布历史的知识单元，类型为文档或媒体。
_Avoid_: Git 仓库、目录、分类

**Document（文档）**：
以 Markdown 表达知识的 Page。
_Avoid_: 附件容器

**Media（媒体）**：
以原始文件表达内容、拥有独立发布历史的 Page。
_Avoid_: 文档附件

**Tag（标签）**：
可同时关联多个 Page 的非层级内容标记。
_Avoid_: 分类、文件夹

**Revision（发布版本）**：
一次成功发布产生的不可变页面快照。
_Avoid_: 草稿、提案快照

**Proposal（提案）**：
请求修改一个 Page 的协作单元，可被审核、合并或关闭。
_Avoid_: 分支、GitHub PR

**Proposal Revision（提案快照）**：
提案某次更新时的不可变候选内容及其基础发布版本。
_Avoid_: 已发布版本

**Review（审核）**：
对明确候选内容和基础版本表达批准、要求修改或评论的意见。
_Avoid_: 合并

**Merge（合并）**：
将经过有效审核的提案内容发布为 Page 新版本的行为。
_Avoid_: 覆盖历史

**Revert（撤销）**：
通过新提案反向撤销某次发布引入的修改，保留其他后续修改。
_Avoid_: 重置、删除提交

**Restore（恢复）**：
通过新提案将页面内容恢复为选定历史快照。
_Avoid_: 改写历史

**Page Issue（页面议题）**：
围绕某个 Page 的问题与讨论，可关联提案。
_Avoid_: GitHub 开发 Issue

**Account（账户）**：
权限和操作归属的主体；人和持有其 Token 的 Agent 共享此身份。
_Avoid_: 独立 Agent 身份

**Page Manager（页面管理者）**：
被明确授权管理某个 Page 的访问和内容治理的账户。
_Avoid_: 自动拥有全部权限的作者
