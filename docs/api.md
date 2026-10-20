# REST 契约

机器契约：[OpenAPI 3.1](../api/openapi.json)。源码为
`scripts/build-contracts.mjs`，修改后运行 `npm run contracts:build`。
检查要求生成结果与源码一致。契约版本 0.1.0 不代表应用已经发布。

## 统一约定

Issue #8 实现 GET /search：q、tag、limit、cursor；拒绝其他查询参数，避免客户端控制
候选权限过滤。返回 SearchHitList，不返回总数、facet 或搜索引擎原始摘要。
当前 head/policy_version 不匹配的候选跳过并安排修复；索引不可用返回 search_unavailable / 503。

- 基础路径 `/api/v1`，JSON UTF-8；ID 使用 UUID，时间 RFC 3339 UTC。
- UUID 输入接受大小写十六进制，Core 统一为小写后比较主体、组成员和幂等操作范围。
- Cookie 会话或 Bearer Token 二选一；若两者同时提供且身份不同，返回
  `invalid_request`。匿名读取仅开放公开对象。
- 变更会话请求校验 Origin 和 X-CSRF-Token；登录、注册校验 Origin，
  会话建立后从登录响应获取 CSRF Token。
- 校验失败使用结构化错误：`error.code`、`message`、`request_id`，
  三方冲突可附 `conflicts`。无权限读取的对象返回 404。
- `limit` 默认 20、最大 100；列表返回 `items` 和 `next_cursor`，
  不提供未经权限校验的总数。游标不透明，绑定调用身份和查询，10 分钟过期。
- 不接受未声明字段；标题 1–300 字符，标签最多 30 个、每个最多 64 字符；
  Markdown 最大 1,048,576 字符。请求另设字节上限以防多字节编码绕过。
- 业务写入提供 Idempotency-Key，作用域为账户与 operation，保留 24 小时；
  同 key 同请求重放已提交结果，不同请求返回 `idempotency_conflict`。
  重放仍重新验证当前账户及读取权限，不返回已不可读数据。
- 认证敏感操作不缓存明文 Token／密码响应；响应丢失后需用户重新生成 Token，
  不能依赖重试重新取得原明文。退出、撤销均允许重复请求。
- 过期 head 或候选返回 409 `stale_revision`；409 不应无条件自动重试。
  网络失败／503 只在幂等保障下重试；429 遵守 Retry-After。

## 版本字段的含义

Issue #5 增加提案创建/更新/状态、明确版本审核、审核历史/diff、rebase、merge 和
revert/restore 的 REST 入口。所有写入要求 Idempotency-Key；nullable head 字段必须
显式提供（首次发布为 null）。审核缺失或有当前有效负面立场返回 review_required；
三方冲突返回 merge_conflict 与 conflicts 的 B/H/P，不写入候选。媒体原件端点仍待 #6，
MCP 目录仍只是目标契约。合并的 outbox 事件已持久化，投递 worker 待 #8。

Issue #4 实现 POST/GET `/pages`、GET `/pages/{id}`、发布历史列表/明确版本、
提案队列/详情/候选历史及标签列表。未发布 Page 的详情仅有元数据与 null revision，
候选内容只在提案视图返回。目录与标签只从可读、未隐藏、已发布 head 生成。
旧版本及候选始终检查当前源 Page ACL；媒体引用的 URL 不授予目标读取权限。
页面创建使用 5 MiB 请求字节上限（同时保留字符上限），最多 1000 个不同媒体引用。
媒体 Page 创建仅接受当前账户上传的 ready blob，管理员可代用；上传入口仍待 #6。
列表使用当前 ACL 过滤后分页，游标绑定调用账户（匿名为独立身份）、查询与页大小。

`expected_head_revision_id` 是调用时看到的当前发布 head，新页面为 null；
`expected_proposal_revision_id` 是调用时看到的候选快照 ID。
rebase 时前者是目标当前 head，候选仍保存旧 base。
更新候选时可以提交相对于当前 head 解决后的完整快照。
审核必须额外绑定 `expected_review_round`。
关闭／重开使用 proposal version，ACL 使用 policy_version，
讨论状态与治理使用各自 version。Page 治理的 expected_version 指 policy_version。

## 媒体与 Agent 上传

Issue #6 实现预约/状态/流式 PUT，以及固定发布版本和明确候选版本的 GET/HEAD 原件读取。
PUT 允许 application/octet-stream 或预约 MIME；必须发送完整原件，不支持分块续传。
预约和 PUT 分别按账户/IP 限流，预约完成后配置降低的大小上限同样约束 PUT。
预约仅本人或管理员可访问，租约并发写返回 invalid_state；超限返回 upload_too_large，
文件头/声明 MIME 不符返回 unsupported_media。失败不登记 ready，预约到期拒绝新 PUT。
ready 后重试仍逐字节摘要校验，不覆盖原件；GET 状态始终按当前账户鉴权。

下载在当前 Page read 权限事务中打开文件，再由 HTTP 处理 Range/条件请求；不存在
按 blob/hash 的公开读取路由。响应 private,no-store、nosniff，并加受限 CSP；
图片/音视频可以 inline，PDF 及显式 download=1 使用 attachment。没有 OCR、转码或前端播放器实现。
已开始传输的文件句柄按本次授权继续，之后的请求（包括 Range、HEAD）重新检查当前 ACL。

1. POST `/uploads` 预约文件名、声明大小和 MIME，返回上传 ID、过期时间及同源路径。
2. PUT `/uploads/{id}/content` 携带同一账户认证，流式传二进制；
   服务端重新校验实际大小、文件头、MIME，不信任声明。
3. 成功返回 ready blob；媒体提案引用 blob_id。GET upload 可确认传输结果。

预约 1 小时有效。重复 PUT 同一原件可返回已有结果；已完成后不同字节返回 409，
不覆盖。只允许上传者或站点管理员查看未关联上传状态。
未完成、失败和超时上传由 GC 按宽限期处理。
MCP 用 upload_prepare 获取路径，用正常 HTTPS PUT 传大文件，不把文件塞入工具 JSON。

原件下载只走发布版本或候选版本内容端点。检查页面与快照关系及当前 read；
支持 Range。媒体响应私有缓存策略，不交由绕过权限的公共静态目录提供。
文档类型调用原件端点返回 validation_failed。

## 管理接口

Issue #7 增加 Page Issue、评论、状态、同 Page 提案关联及 Page/Issue/评论 moderation。
讨论正文最多 65,536 字符，标题最多 300 字符；创建/评论请求上限 1 MiB，以支持多字节
文本及 JSON 转义。创建和评论分别按账户/IP 限流，所有业务写入要求 Idempotency-Key。
issues_list、issue_get、comments_list 支持 include_hidden=true 的管理视图；普通读取不返回隐藏对象。
游标绑定当前身份、源 Page/Issue、状态、管理模式和页大小；先鉴权与过滤再分页。
幂等重放也检查当前 Page 与讨论可见性，隐藏后的正文不从旧响应缓存返回。
moderation 要求 reason 与 expected_version；Page 使用 policy_version，Issue/评论使用各自 version。
议题标题和治理理由的字符上限同时约束 NFC 规范化后的值；超限统一返回 validation_failed。

Issue #3 实现 `/auth/*`、账户/用户组/审计管理及页面 policy 接口；页面内容、历史、
提案、讨论与下载仍由后续 Issue 提供。ACL 可以对已存在的页面策略操作；本阶段无公开
的空页面创建入口。页面创建用例将在 #4/#5 的事务中同时写入页面与策略。

Cookie 名为 `wikit_session`；成功登录返回 account/csrf_token 并设置 Cookie。
注册仅返回 account/token，不自动建立会话。敏感操作要求有效会话及当前密码；
Bearer 不能调用。Cookie 与 Bearer 同时出现时两者均须有效并属于同一账户，
会话变更仍校验 CSRF。Origin 必须等于配置的公共 origin。
退出允许重复，清除 Cookie；提供了失效 Cookie 的退出仍须通过 Origin 检查。
请求体限制 64 KiB，拒绝未知字段和尾随 JSON；错误与敏感响应均使用 no-store。

账户管理读结果包含 auth_version；角色/禁用变更递增代次并立即使旧会话失效，
Bearer 通过每次读取当前状态即时生效。Token 管理返回创建时间，但不会返回旧明文。
管理写入使用 Idempotency-Key，24 小时内按账户/操作/请求摘要去重，重放先重新鉴权。
列表游标为随机服务端记录，绑定账户、列表类型及查询，10 分钟有效。

账户／用户组／审计操作仅管理员可执行；不允许禁用或降级最后一个活跃管理员。
页面策略采用整体替换，Core 校验调用者可授予范围，不能通过一个 PUT 扩大自身权限。
审核队列、页面列表和讨论列表使用统一读权限过滤。
缺少公开部署能力的管理命令不伪装成 HTTP 端点；见部署手册。
