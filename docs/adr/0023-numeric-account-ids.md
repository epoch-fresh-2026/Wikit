# 0023 递增账户 ID

状态：accepted；日期：2026-10-22。

Wikit 是单站点单数据库，不需要离线或跨实例生成账户身份。账户 ID 改用 PostgreSQL
分配的正整数 bigint，从 1 起、允许间隙、不复用；其紧凑性和人工排查便利优于当前
UUID 的独立生成能力。ID 可枚举不改变授权要求，用户名和角色不由编号推导。
REST/MCP 与 Go/Web DTO 均传规范十进制字符串，范围 1–9223372036854775807，
拒绝前导零、符号、空白、非数字及溢出。其他 UUID（包括组）不变。

用户随后明确“不考虑迁移，破坏性更新”：直接修改新部署 SQL 基线，不实现旧 UUID
数据转换或兼容层。旧库因迁移历史校验和不同被拒绝，必须显式重新初始化空数据库，
程序不得自动清空旧库。旧数据库与旧版本可保留配套备份，不支持跨基线直接升级或回退。
全部账户外键使用 bigint；page_grants.subject_id 为 text，按 subject_type 区分数字用户
与 UUID 组。Core 维护独立账户 ID 校验，身份认证、授权与审计归属语义不变。

影响范围：users、user_tokens、sessions、group_members、page_grants、blobs、uploads、
proposals、revisions（作者与发布者）、page_issues、comments、moderation_events、
audit_events、idempotency_records、list_cursors，以及 REST/MCP 契约、前端生成类型和测试。
