# 单机部署、备份与恢复

状态：已提供后端基础、账户授权、提案发布和本地媒体持久化；产品部署仍为实现目标，无正式镜像。
部署阶段须提交真实 Dockerfile、compose.yaml、环境示例与 smoke 测试后才能发布。

## 目标拓扑

Compose 包含 wikit、PostgreSQL、Meilisearch。只向宿主暴露应用端口；
数据库与检索通过内部网络访问。生产入口由维护者的反向代理终止 TLS。
应用容器非 root，前端静态资源内嵌于发布产物。

持久卷：PostgreSQL 数据、媒体原件、Meilisearch 数据。
搜索卷丢失可重建，数据库与原件丢失不可仅靠其他卷恢复。
镜像及基础镜像在实现阶段锁定明确版本／digest。

| 配置 | 默认／要求 |
| --- | --- |
| WIKIT_PUBLIC_URL | 必填，生产 HTTPS，同源 REST/MCP |
| WIKIT_DATABASE_URL | 必填，秘密，应用专用数据库账户 |
| WIKIT_MEILI_URL | serve/search-rebuild 必填，内部 HTTP(S) origin |
| WIKIT_MEILI_KEY | 必填，秘密，不进入前端构建 |
| WIKIT_MEDIA_ROOT | `/var/lib/wikit/media`，持久卷 |
| WIKIT_MAX_UPLOAD_BYTES | 104857600（100 MiB） |
| WIKIT_BLOB_GC_GRACE | 168h |
| WIKIT_REGISTRATION_ENABLED | true |
| WIKIT_SESSION_TTL | 24h |
| WIKIT_LISTEN_ADDR | `:8080` |

秘密通过部署环境或 secrets 文件注入，不提交仓库。
启动时检查关键配置与连接。`/health/live` 表示进程可运行；
`/health/ready` 检查数据库、迁移兼容性和媒体写入能力。
搜索不可用标记降级并告警，不阻止数据库读写服务；搜索 API 返回 503。

健康端点：GET `/health/live` 返回进程状态，GET `/health/ready`
检查数据库及精确迁移历史、媒体目录临时文件写入／同步／删除。
失败只返回通用 unavailable，不暴露连接信息；检查 context 超时为 2 秒。
本地文件系统调用不能被 context 中断，故不承诺故障磁盘下的硬性响应时限。
启动先验证上述依赖，失败则不监听。`WIKIT_PUBLIC_URL` 接受 HTTPS 或仅本机
HTTP（localhost／loopback IP），禁止凭证、查询和 fragment；监听地址默认 `:8080`。
账户阶段已实现 WIKIT_REGISTRATION_ENABLED 与 WIKIT_SESSION_TTL；
serve 要求至少一名活跃管理员。Issue #8 起 serve/search-rebuild 要求 Meilisearch URL 和至少 16 字节的 key，
禁止 URL 携带凭证、路径、查询或 fragment；不会因为 Meilisearch 暂不可用而拒绝数据库服务启动。

## 初始化与升级

Issue #8 内置每秒检查的搜索 worker；单轮有 30 秒时限，异步 task UID 和重试状态持久化。
`wikit search-rebuild` 构建新索引、追赶增量后切换数据库 active_index；失败时旧指针保留。
`wikit search-retry` 将失败 outbox 的下一次尝试设为立即；`wikit search-status` 输出
pending、oldest_seconds、failures，不输出正文、ACL 或 key，且状态读取不等待 worker 锁。
retry/status 只要求数据库配置。重建和投递共用会话锁，正在执行时另一管理写命令返回非零；可稍后重试。
Meilisearch 固定版本 1.55.0，管理 key 仅供服务端使用；索引卷丢失后必须执行完整 search-rebuild。
旧/失败重建索引保留，清理前核对 search_state.active_index；不会自动删除线上索引。

Issue #6 提供 `wikit blob-gc`；读取数据库、媒体目录和 WIKIT_BLOB_GC_GRACE，默认 168h，
只清理超过宽限期的未引用 blob、孤立原件和临时文件，输出数量而非物理路径。
失败返回非零，可重复运行恢复 deleting 墓碑。备份窗口停止上传与 GC。
serve 读取 WIKIT_MAX_UPLOAD_BYTES（默认 104857600）和同一宽限期配置；
单次 PUT 有 15 分钟请求时限，数据库写租约为 30 分钟，完成前会重新检查预约与账户。
上传上限配置范围 1 字节～1 TiB，GC 宽限期至少 1 小时；默认仍为 100 MiB 与 7 天。
blob 删除意图与完成状态写入审计，actor 为本地系统、request_id 为本次 GC 的随机运行 ID。
媒体根目录由部署者独占管理，子目录为 objects 和 staging；文件名只使用服务端 UUID。

实现阶段提供独立 migrate、create-admin、search-rebuild、blob-gc 管理命令。
操作必须输出明确结果，凭证交互输入，不作为命令参数。

首次部署：准备环境与卷 → 启动依赖 → migration → 本地创建管理员 →
启动应用 → 验证注册、创建提案、审核合并、媒体和搜索。
升级：阅读 schema 兼容声明 → 备份 → 拉取固定版本／digest →
执行迁移 → 更新应用 → smoke 验证。不要在初始化缺失时开放公共注册入口。

如果旧应用兼容新 schema，可切回旧镜像；否则恢复匹配的数据库和媒体备份。
数据库回退不是简单回滚容器，发行说明必须说明兼容窗口与数据损失边界。

## 一致备份

首版采用维护窗口方案，优先可验证性：

1. 停止应用写入、上传、worker 与 GC，等待在途事务和文件提交结束。
2. 生成 PostgreSQL 一致性逻辑备份，记录 schema 版本和应用 digest。
3. 复制媒体原件卷并生成校验清单。备份期间不运行 GC。
4. 检查数据库中所有可达 blob 都在备份，记录时间与校验结果。
5. 恢复服务。备份放在独立存储，不与唯一生产磁盘共损。

恢复：空环境恢复数据库与原件 → 验证文件校验及可达性 →
运行匹配版本应用 → 重建搜索 → 执行权限、历史、下载、提案 smoke。
会话／Token 备份恢复可能恢复旧凭证状态；恢复流程默认整体撤销会话和用户 Token，
用户重新登录后生成 Token。管理员凭证可用性须在恢复演练中验证。

## 监控

结构化日志带 request_id，不含内容或凭证。记录请求延迟／错误率、数据库连接、
磁盘空间、outbox 积压及最旧事件年龄、索引失败、上传失败、备份时间。
容量告警和备份频率由站点维护者配置；首版不提供通知系统。
至少在每次涉及持久化的版本发布前执行一次恢复演练。
