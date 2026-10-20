# Meilisearch 检索，PostgreSQL 决定权限

中英文全文检索采用 Meilisearch，以获得专门的中文分词和相关性能力。
PostgreSQL simple/pg_trgm 不能直接满足该目标；自研分词和索引会扩大首版范围。

代价是异步索引和额外运维。索引可重建，所有结果由 Core 校验当前权限及版本；
宁可在索引延迟时暂时少返回，也不能暴露旧权限允许读取的内容。
