# Wikit

当前发布规则见 [ADR 0018](docs/adr/0018-wiki-publication-redesign.md)（Issue #35）。

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
请求修改一个 Page 的协作单元，可检查差异、合并或关闭。
_Avoid_: 分支、GitHub PR

**Proposal Revision（提案快照）**：
提案某次更新时的不可变候选内容及其基础发布版本。
_Avoid_: 已发布版本

**Merge（合并）**：
有权限的账户检查差异后，将提案内容发布为 Page 新版本的行为。
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

**Moderator（版主）**：
具有普通公开页面合并能力的全站角色 `moderator`；受限页需显式授权，首页只允许管理员合并。

**创建权限（create_page）**：
管理员授予账户或用户组的独立能力，允许直接发布新页面首版；新账户默认不具备。

Page 使用递增正整数 ID，REST/MCP 以十进制字符串传输。其他对象保留 UUID。
首页固定为 Page 1，是公开文档，不能删除、隐藏、替换身份或改为受限。普通用户可提出修改。
