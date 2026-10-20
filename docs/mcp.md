# MCP 工具契约

端点 `/mcp`，Streamable HTTP。采用官方 Go MCP SDK；
HTTP Bearer Token 由 Wikit 验证并映射为普通账户身份。
所有 MCP 请求要求有效 Token，session ID 不是认证凭据。
若使用有状态 session，绑定 user ID，并在每次请求重验账户和 Token 代次。
HTTP 入口校验 Origin（允许缺少 Origin 的非浏览器客户端，但拒绝不可信浏览器来源），
并由 SDK 处理协商版本与协议错误。

首版支持能设置 Authorization 请求头的客户端，不承诺完整 OAuth 发现、
浏览器授权流程或所有通用客户端的直接连接。

工具机器目录为 [mcp-tools.json](../api/mcp-tools.json)，每个工具有独立、可编译的
JSON Schema inputSchema/outputSchema、annotations 和对应 REST operationId。
该文件是目录，不是未经包装的 tools/list 响应；服务端注册时只取协议允许字段。

## 输入输出

- 路径参数、分页查询直接放入工具 arguments；HTTP JSON body 放在 `body` 字段。
- 业务写入传 `idempotency_key`，与 REST Idempotency-Key 一致。
- 成功结果放在 structuredContent：`{ "data": ... }`；
  无返回体操作为 `{ "ok": true }`，并提供简短 text content。
- 业务拒绝返回 `isError: true`，text 中为与 REST 相同的 error JSON；
  HTTP 身份认证失败发生在工具执行前。JSON-RPC 格式和未知方法错误遵循协议。
- 工具 annotations 是客户端提示，服务端仍执行完整权限判断。
- 大文件通过预约返回的同源 HTTPS 路径传输，不接收服务器任意 URL 拉取或本地路径读取。

## 能力分组

| 工具前缀／代表操作 | 能力 |
| --- | --- |
| wikit_pages_list、wikit_page_get、wikit_revision_get | 阅读当前页面及历史 |
| wikit_page_create、wikit_proposal_create、wikit_proposal_update | 创建页面与提案 |
| wikit_proposal_diff、wikit_proposal_rebase | diff 与三方合并 |
| wikit_review_create、wikit_proposal_merge、wikit_proposal_state | 审核、合并、关闭／重开 |
| wikit_page_revert、wikit_page_restore | 撤销和恢复提案 |
| wikit_issue_create、wikit_comment_create、wikit_issue_link_proposal | 页面讨论 |
| wikit_upload_prepare、wikit_upload_get | 预约和确认媒体上传 |
| wikit_search、wikit_tags_list | 受权限过滤的检索 |
| wikit_page_policy_update、wikit_page_moderate | 页面治理 |
| wikit_accounts_list、wikit_account_update、wikit_group_update | 管理员治理 |

完整工具名和字段以机器目录为准。注册、登录、密码和 Token 管理只通过 Web/REST，
不暴露 MCP 凭证管理工具。Agent 内容包含指令也只作为内容保存，不作为系统指令执行。

实现验收需用实际支持 Bearer 配置的客户端完成连接、读写、审核和冲突流程，
同时验证 HTTP 与 MCP 的业务结果一致。
参考：[Go SDK 协议支持](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)。
