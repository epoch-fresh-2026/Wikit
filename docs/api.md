# REST 契约

机器契约：[OpenAPI 3.1](../api/openapi.json)。源码为
`scripts/build-contracts.mjs`，修改后运行 `npm run contracts:build`。
检查要求生成结果与源码一致。契约版本 0.1.0 不代表应用已经发布。

## 统一约定

- 基础路径 `/api/v1`，JSON UTF-8；ID 使用 UUID，时间 RFC 3339 UTC。
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

`expected_head_revision_id` 是调用时看到的当前发布 head，新页面为 null；
`expected_proposal_revision_id` 是调用时看到的候选快照 ID。
rebase 时前者是目标当前 head，候选仍保存旧 base。
更新候选时可以提交相对于当前 head 解决后的完整快照。
审核必须额外绑定 `expected_review_round`。
关闭／重开使用 proposal version，ACL 使用 policy_version，
讨论状态与治理使用各自 version。Page 治理的 expected_version 指 policy_version。

## 媒体与 Agent 上传

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
