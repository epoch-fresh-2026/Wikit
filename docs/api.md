# REST 契约

当前发布规则见 [ADR 0018](adr/0018-wiki-publication-redesign.md)（Issue #35）。

机器契约：[OpenAPI 3.1](../api/openapi.json)。源码为
`scripts/build-contracts.mjs`，修改后运行 `npm run contracts:build`。
检查要求生成结果与源码一致。契约版本 0.1.0 不代表应用已经发布。

## 统一约定

Issue #9 增加 /mcp 的 39 个目录工具，直接复用既有 Core。REST/MCP 的 JSON body 幂等摘要
忽略对象字段顺序和无意义空白，保留数字词法；旧原始字节摘要仍兼容原 body 字节的重试。
稳定错误 DTO 共用，工具业务拒绝通过 isError 与同一 error JSON 返回；协议身份失败在执行前处理。

Issue #8 实现 GET /search：q、tag、limit、cursor；拒绝其他查询参数，避免客户端控制
候选权限过滤。返回 SearchHitList，不返回总数、facet 或搜索引擎原始摘要。
当前 head/policy_version 不匹配的候选跳过并安排修复；索引不可用返回 search_unavailable / 503。

- 基础路径 `/api/v1`，JSON UTF-8；Page ID 使用正整数十进制字符串，其余 ID 使用 UUID，时间 RFC 3339 UTC。
- UUID 输入接受大小写十六进制，Core 统一为小写后比较主体、组成员和幂等操作范围。
- Cookie 会话或 Bearer Token 二选一；若两者同时提供且身份不同，返回
  `invalid_request`。匿名读取仅开放公开对象。
- 变更会话请求校验 Origin 和 X-CSRF-Token；登录、注册校验 Origin，
  会话建立后从登录响应或同源恢复接口获取 CSRF Token。
- 校验失败使用结构化错误：`error.code`、`message`、`request_id`，
  三方冲突可附 `conflicts`。无权限读取的对象返回 404。
- `limit` 默认 20、最大 100；列表返回 `items` 和 `next_cursor`，
  不提供未经权限校验的总数。游标不透明，绑定调用身份和查询，10 分钟过期。
- 页面目录按数值 Page ID 升序，以最后消费候选为游标锚点，每请求最多检查 1,000 个候选。
  权限过滤后允许不足一页或空页带 next_cursor；有游标不代表后续一定存在可见结果。
  翻页重新鉴权，不提供跨请求快照；迁移前的页面目录游标失效，从第一页重试（Issue #49）。
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

`POST /pages` 检查 create_page 并原子发布首版，返回 `{page, revision}`，不返回 proposal。
会话账户信息返回有效 can_create_page，管理账户/组契约使用 create_page 配置授权。
Page 1 由部署创建，不允许隐藏、替换、改为受限；只有管理员可合并其修改。
PageDetail 包含当前有效 capabilities。前端显示入口，服务端每次操作重新鉴权。
目录/标签只从可读、未隐藏的发布 head 生成，候选只在提案接口返回。

提案创建、更新、状态、diff、rebase、merge、revert/restore 均复用 Core。
删除 reviews 读写接口及 Review/ReviewInput/ReviewList 类型，无 review_round 或 review_required。
`expected_head_revision_id` 必须显式提供并匹配读取时的当前发布 head，
`expected_proposal_revision_id` 必须匹配读取时的候选。新部署页面创建已发布，提案有实际基础版本。
rebase 使用目标当前 head；手工冲突解决提交完整快照。版本变化返回 stale_revision，必须重新读取差异。
三方冲突返回 merge_conflict 与 B/H/P 内容，不保存有冲突标记的候选。
关闭/重开使用 proposal version，ACL 使用 policy_version；治理使用 expected_version。

页面创建请求上限 5 MiB，最多 1000 个不同媒体引用；媒体创建只接受本人或管理员可用的 ready blob。
历史、候选及下载始终验证当前源 Page ACL，固定媒体 URL 不授予读取权限。

## 媒体与 Agent 上传

Issue #6 实现预约/状态/流式 PUT，以及固定发布版本和明确候选版本的 GET/HEAD 原件读取。
PUT 允许 application/octet-stream 或预约 MIME；必须发送完整原件，不支持分块续传。
预约和 PUT 分别按账户/IP 限流，预约完成后配置降低的大小上限同样约束 PUT。
预约仅本人或管理员可访问，租约并发写返回 invalid_state；超限返回 upload_too_large，
文件头/声明 MIME 不符返回 unsupported_media。失败不登记 ready，预约到期拒绝新 PUT。
ready 后重试仍逐字节摘要校验，不覆盖原件；GET 状态始终按当前账户鉴权。

下载在当前 Page read 权限事务中打开文件，再由 HTTP 处理 Range/条件请求；不存在
按 blob/hash 的公开读取路由。响应 private,no-store、nosniff，并加受限 CSP；
图片/音视频可以 inline，PDF 及显式 download=1 使用 attachment。没有 OCR 或服务端转码。
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
提案列表、页面列表和讨论列表使用统一读权限过滤。
缺少公开部署能力的管理命令不伪装成 HTTP 端点；见部署手册。


### 会话写入凭证恢复（Issue #47）

`POST /auth/session` 接受严格 JSON `{}`，仅有效 Cookie 会话可用，拒绝 Bearer（包括混用）。
必须携带与公共 origin 完全相同的 Origin，无需旧 CSRF；不提供 CORS 或 MCP 入口。
成功返回 Session（account/csrf_token），Cache-Control: no-store，不设置 Cookie、不延长期限。
Core 在同一授权事务检查当前账户、会话及有效创建能力；失效、禁用、代次不符返回 401，
缺少 Origin 返回 403。普通变更请求仍必须同时满足 Origin 与 CSRF。
凭证按会话稳定派生，跨标签页恢复不互相失效；旧版本随机 CSRF 仍可用于原会话。
