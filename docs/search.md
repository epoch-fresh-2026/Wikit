# 中英文检索与权限一致性

Meilisearch 是查询加速器，不能决定对象是否可读。
PostgreSQL 的 simple 字典和 pg_trgm 不作为中文分词替代。

Issue #8 使用 Meilisearch 1.55.0，HTTP 适配器仅处理服务协议；Core 定义候选、
当前权限/版本复核、摘要及 outbox 用例。不增加 SDK 依赖。searchableAttributes 固定为
title、tags、body，attribute 排序优先，文本与查询使用 zho/eng locales。
查询拒绝空白、NUL、非法 UTF-8 和超过 500 字符；标签按现有 NFC/case-fold 规则处理。
摘要只从 PostgreSQL 当前发布内容提取纯文本，最多 2,000 字符，不返回 HTML 高亮。

## 本阶段实现边界

0007 migration 扩展既有 outbox：递增 sequence、可空 revision、持久 task UID、
重试时间与无正文错误代码。Page 策略修改的数据库触发器在业务事务内追加事件，
覆盖 ACL、隐藏/恢复；组成员变更由每次查询的当前成员关系过滤，无须把成员复制进索引。
旧事件只读取数据库当前投影，不重放历史正文。版本不匹配的搜索候选会追加去重修复事件。

worker 和 search-rebuild 共用 PostgreSQL 会话 advisory lock；首版串行处理全部事件，
因此同 Page 事件不会超车。每个事件提交 task UID 后才轮询，重启继续等待同一 task；
只有 succeeded 才完成事件。失败持久指数退避（1 秒至 5 分钟），任务失败后清除 UID 重新投递。
网络/等待超时保留已知 task UID；错误只记录固定代码，不保存服务响应或正文。
search-retry 命令使失败事件立即可重试；search-status 输出积压量、最旧年龄、累计失败次数。
serve 内 worker 降级重试，检索故障不阻止数据库读写和 readiness。

全量重建使用独立索引名：导出当前已发布投影并记录 outbox 水位，等待全部写入 task，
再把水位之后的 Page 变化投影到新索引。成功后原子切换 PostgreSQL 中的 active_index，
避免依赖远程 swap 和本地事务的双写原子性；失败或进程中断保留旧指针。
水位读取短暂取得业务授权锁，避免未提交事务先分配 sequence 后提交而落在水位之前；网络工作不持有该锁。
worker 在切换后继续消费未完成事件，最后追赶水位之后的新变化允许短暂索引延迟。
旧索引保留供运维排查，孤立重建索引可按 active_index 核对后删除，不自动清除。
重建同时按 JSON 字节预算和文档数量切分批次；超过批次目标的单文档单独发送，
避免多篇合法长文经 JSON 转义后合并成超过搜索服务请求上限的批次。
游标绑定索引代次，重建切换后旧游标返回 invalid_request，客户端从第一页重试。
重建与正常投递均读取当前投影，不允许迟到事件恢复旧正文或隐藏页面。

每次最多检查 1,000 个候选，游标记录实际消费位置，候选不足时可以返回不足一页。
Meilisearch 只返回 ID/revision/policy_version；客户端提供的 filter/facets 等非契约参数被拒绝。
候选过滤仅优化召回，最终响应仍在当前授权事务中重新解析账户、组、策略和 head。

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

Issue #49：每批按需取得候选，批量读取 Page/head 与完整 ACL，足够一页即停止。
后续网络请求不持业务锁；跨批候选在最终响应事务重新验证，不能沿用先前批次的授权或正文。

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
