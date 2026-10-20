# 内嵌 SQL 与事务迁移基础

状态：accepted；日期：2026-10-20；对应已确认设计的 Issue #2。

在 PostgreSQL 版本引擎基线上，采用内嵌、连续编号的 SQL 文件与 SHA-256 历史，
由独立 migrate 命令执行。一个事务持有固定 advisory lock，原子提交全部待执行
迁移及历史；不随服务启动自动升级，不提供生产 down。

当前只需 PostgreSQL、单一线性版本和事务 DDL，因此使用 pgx 实现小型专用 runner，
不引入跨数据库迁移框架。代价是不能运行 CREATE INDEX CONCURRENTLY 等事务外操作；
如未来需要在线大表迁移，应以单独 Issue 修订此决策和恢复策略。

首个迁移只建立业务 schema，业务表按后续 Issue 逐步落地。校验和差异、历史缺口
与未知新版本均拒绝执行；启动与 readiness 要求精确匹配。迁移失败整批回滚后可重试，
不声称应用已兼容尚未实现的业务模型。不同平台的 SQL checkout 强制使用 LF。
