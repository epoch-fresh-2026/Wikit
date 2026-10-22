# WebUI

当前发布规则见 [ADR 0018](adr/0018-wiki-publication-redesign.md)（Issue #35）。

Issue #10 使用 React、TypeScript strict、Vite、Tailwind CSS 与 shadcn/ui；按阅读/搜索、
编辑、提案合并、讨论、账户和治理 feature 分组。API 类型从 OpenAPI 生成，统一客户端负责
同源 Cookie、CSRF、稳定错误及幂等 key，不在 UI 重写权限或发布状态机。

## 页面与闭环

前台为 Wiki React SPA：顶部站名（链接首页）、搜索和账户菜单，中间正文、右侧文章目录。
创建入口只在非空关键词、无标签或类型过滤的搜索成功返回零结果后，向具有 can_create_page 的账户显示；
加载、失败、空搜索和已有结果时不显示。链接携带搜索词，在新建表单预填标题（Issue #41）。
按 Issue #39 与 ADR 0019 移除站点侧边栏、全部页面与全站提案页面；页面顶部阅读、讨论、历史、提案等操作右对齐，窄屏换行。
窄屏文章目录使用 Sheet 按需打开。首页 `/` 读取 Page 1，与普通文档共用阅读与提案流程。
不使用宣传标题、装饰英文眉题或口号。后台采用独立 AdminLayout，以横向导航提供账户、用户组、审计及返回站点入口。

| 路由 | 内容 |
| --- | --- |
| `/` | 固定首页 Page 1 |
| `/search` | 搜索、标签与类型过滤；无关键词时列出匹配页面 |
| `/pages/new` | 具有 create_page 时创建并直接发布文档或媒体 |
| `/pages/:id` | 阅读、标签、当前版本、页面工具 |
| `/pages/:id/history`、`/pages/:id/revisions/:revisionID` | 历史、比较、恢复与撤销提案 |
| `/pages/:id/edit` | 为已有页面提出修改 |
| `/pages/:id/proposals` | 该页面的提案列表 |
| `/pages/:pageId/proposals/:id`、其 `/edit` | 独立提案详情与编辑，校验所属 Page |
| `/pages/:id/issues`、`/pages/:pageId/issues/:id` | 页面讨论列表及独立讨论详情 |
| `/pages/:id/policy` | 页面 ACL 与治理，首页不能隐藏或限制访问 |
| `/admin/accounts`、`/admin/groups`、`/admin/audit` | 账户角色/状态/创建权、组成员/创建权、审计 |

`/admin` 默认进入账户管理。新建成功进入已发布页面；修改已有页面进入提案详情。
`/pages` 与 `/proposals` 不再注册为前台路由，旧地址显示未找到页面；标签链接统一进入 `/search?tag=…`。
Page 之间没有所属或父子层级；所有页面均不显示面包屑，首页通过顶栏站名访问（Issue #43）。
提案列表仅在具体 Page 下提供；REST/MCP 列表接口不变。
提案默认展示差异，候选预览和候选历史通过 Tabs 切换，编辑使用独立路由。
有有效 merge 能力且正在展示匹配当前 head/候选的 diff 时可合并，无批准表单、审核记录或轮次。
候选或 head 变化后必须重新读取；不会自动发布新的内容。恢复和撤销仍生成提案。
过期提案的独立编辑页必须读取并展示基础和当前发布版本，编辑者确认已结合当前发布内容解决后才能保存；提交仍绑定所展示的 head 和候选。
冲突字段 ours 是当前发布，theirs 是待合入内容。用户组成员分页、重试等辅助按钮不提交外层表单。
页面工具按 Core 返回的 capabilities 展示操作；会话账户返回有效 can_create_page。

官方 shadcn CLI 固定为 4.22.1，Radix/new-york/neutral 配置，组件源码和依赖锁提交仓库。
复用 Button、Input、Textarea、Select、Checkbox、Badge、Tabs、
Table、AlertDialog、DropdownMenu、Sheet、Alert、Skeleton 等组件。业务代码仅组合控件和内容排版。
标签使用 Input、Badge、Button：Enter 形成标签、逐项删除，不按逗号或空格切分；
中文 IME 确认不添加标签或提交表单。表单提交包括尚未确认的非空输入。
Core 最终执行 NFC/case-fold、去重、数量与长度校验。用户组成员用账户选择控件。

## 状态与失败

认证使用 HttpOnly 会话 Cookie。仅将不能独立授权的 CSRF nonce 存于 sessionStorage，
支持同一标签刷新；密码、个人 Token、内容草稿和响应不持久化。缺少 CSRF 时提示重新登录。
编辑内容未提交时，SPA 导航及浏览器离开均提示；草稿只在内存中。
身份变化或 401 清空内容和草稿子树，取消在途读取；路由变化不显示旧对象数据。
客户端不缓存受限响应、不使用 Service Worker；404 不展示之前读取的正文或标题。

写请求在网络失败后保留相同请求的幂等 key；修改请求内容后使用新 key。业务拒绝明确展示
稳定中文错误及 request ID，三方冲突另展示字段值。网络和基础设施诊断不原样显示。
错误提示组合官方 Alert 的直接图标子元素、AlertTitle 和 AlertDescription；正文完整换行，
请求编号、重新登录链接与冲突内容位于说明区域，窄屏不重叠或被挤成单字列（Issue #37）。
上传先预约再向固定同源路径 PUT 原件，成功后使用 blob ID；不读取任意服务器 URL。

表单有可见标签，按钮与链接可通过键盘操作，状态使用 live region，错误使用 alert；
路由切换将焦点移到主内容。窄屏改为单列，长 ID、diff 和 Markdown 代码区可横向滚动。
长标题、标签及账户名称不得撑宽页面或遮挡操作：正文与标签允许完整换行，顶栏账户名在可用宽度内截断，
完整名称通过账户入口保留。历史操作栏可换行；表格与代码仅在各自容器滚动。
文章目录在屏幕高度内独立滚动，窄屏 Sheet 保留固定标题和关闭入口；选择目录项后将焦点与滚动位置移到对应标题，
直接关闭或 Escape 则仍回到目录按钮。音频控件不超出正文宽度。

## 开发与验证

先 `npm ci` 安装根目录契约生成器，再 `npm ci --prefix web`，然后运行 `npm run dev --prefix web`。浏览器访问 127.0.0.1:5173；
后端使用 WIKIT_PUBLIC_URL=http://127.0.0.1:5173、监听 127.0.0.1:8080，Vite 仅代理 /api 和 /health。
代理保留浏览器 Origin，不通过改写 Origin 绕过 CSRF。生产 dist 的嵌入及 Compose 由 #11 完成。

功能路由延迟加载，加载失败提供重新加载入口。必须通过 typecheck、组件测试和生产构建；
frontend CI 随实现启用。组件测试覆盖用户行为、
契约请求、CSRF、错误/冲突与草稿保留、安全 Markdown、取消读取、身份隔离及键盘操作。
模拟 API 的组件测试不算真实后端端到端 smoke；实际运行的范围在 Issue/PR 中单独记录。

Issue #33 升级 jsdom 30 后，根目录与 Web 开发环境最低要求 Node 24.15.0（<25）；Docker 构建同步固定该版本。

Issue #39 本地验证（2026-10-22）：`npm run check`、前端类型检查、42 项组件测试与生产构建、
`go vet ./...` 和 `go test -race -count=1 ./...` 通过。
独立内存 API 夹具加载生产构建，在 320、768、1280 CSS 像素检查阅读页：页面操作均为右对齐，
窄屏换行且页面无横向溢出；320 像素搜索与后台账户页也无横向溢出。
此浏览器验证不代表真实后端端到端测试。本轮未运行 PostgreSQL/Meilisearch 集成及容器验收：
本机未配置本轮集成依赖且没有 Docker；全部 CI 阶段保持启用。
