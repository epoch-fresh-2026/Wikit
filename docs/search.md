# 中英文检索与权限一致性

Meilisearch 是查询加速器，不能决定对象是否可读。
PostgreSQL 的 simple 字典和 pg_trgm 不作为中文分词替代。

## 索引

每个已发布、未隐藏 Page 一条索引记录：
page_id、revision_id、policy_version、type、title、tags、
Markdown 提取的纯文本／媒体描述、visibility、允许读取的用户／组标识。
title 权重优先，其次 tags，再正文。配置 zh/en 文本分析及查询语言一致性，
使用确定的中文测试语料验收，不承诺跨语言语义召回。

不索引未发布候选、历史正文、Issue、评论、原始二进制、密码或 Token。
Meilisearch 仅内部网络可访问，管理 key 不交给浏览器或 MCP 客户端。

## 读取流程

1. Core 从当前账户、用户组构造权限候选过滤，不能信任客户端传入的 ACL。
2. 搜索索引只返回候选 ID、版本与排序信息。
3. 在 PostgreSQL 批量复核当前账户、Page read、隐藏状态与 head。
4. 仅使用复核后的当前标题／正文生成摘要；版本不匹配的候选本轮跳过并安排修复。
5. 继续取候选直到达到 limit 或每次最多检查 1,000 个候选。
   返回结果及不透明 next_cursor，不返回搜索引擎的总命中数、facet 计数或原始摘要。

分页允许因过滤不足一页，next_cursor 不证明还有可见命中。
游标绑定用户、查询及过期时间；复用游标不能绕过重新鉴权。
标签浏览使用数据库中当前可读已发布 Page 的聚合，不枚举全站受限标签。

权限收回即时阻止结果返回；新授权和新内容可能在异步索引完成前暂未被搜到。
短中文、空查询、特殊字符、分页和最大查询长度都需验收。

## 更新与故障

业务事务写 outbox，worker 按 Page 顺序处理，成功完成 Meilisearch task 后确认。
重复投递安全；失败指数退避并记录无正文的错误摘要，管理员可重试／重建。
删除或隐藏事件也要等待搜索删除任务完成，但 Core 鉴权不等待它。
索引不可用时搜索返回 `search_unavailable` / 503，不返回未校验缓存。
阅读页面、提交提案和合并仍可使用数据库。

重建从数据库导出当前已发布可索引 Page，建立新索引，追赶增量后切换。
切换期间所有结果仍由 Core 复核。记录索引积压量、最旧事件年龄和失败次数。

参考：[Meilisearch 分词](https://github.com/meilisearch/documentation/blob/main/capabilities/indexing/advanced/tokenization.mdx)、
[异步索引](https://github.com/meilisearch/documentation/blob/main/capabilities/indexing/overview.mdx)。
