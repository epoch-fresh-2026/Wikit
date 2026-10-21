# v0.1.0 发布验收记录

日期：2026-10-21。Issue #11，交付 PR #31。#1–#10 均已关闭，#3 的跨 Issue
账户/授权验收已在 #9 阶段补齐。PR #31 已审查修复并合并；正式发布执行记录见下文。

## 可复核证据

完整执行：[CI 39545813882](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39545813882)，
源提交 `2c05134e78f31c03d54ebf584ac5fd13b0af8582`。文档/契约、Go vet/race、WebUI
类型同步/strict/28 项组件测试/构建、真实 PostgreSQL/Meilisearch 集成、Container and Compose
及 CI gate 全部通过。另有前一完整通过的
[CI 39544971393](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39544971393)。

容器命令为 `docker build --tag wikit:ci .` 和 `bash scripts/smoke.sh`。
恢复及基准使用真实服务；前端交互由组件测试覆盖，容器另验证生产静态资源和 SPA/API 边界。
本机没有 Docker CLI，容器证据来自上述 Linux CI。早期被新提交替换的运行不计为完整通过。

| 验收 | 执行结果 |
| --- | --- |
| 空环境 | 新建独立 Compose project/卷，显式迁移、初始化管理员、启动应用 |
| 镜像 | 多阶段构建，前端内嵌，应用 UID/GID 65532；只暴露应用端口；应用数据库角色无 superuser/createdb/createrole |
| 发布闭环 | 真实注册、Token、预约上传、提案审核/合并，20 次不可变发布及固定媒体引用 |
| 一致备份 | 停止应用/worker，退出码正常；pg_dump、原件归档、逐文件及可达原件 SHA-256/大小、迁移历史和镜像清单 |
| 空卷恢复 | 数据库/原件还原；拒绝损坏归档与非空目标；撤销旧 Cookie/Token、幂等响应和游标；清理旧远端任务编号 |
| 恢复验证 | 管理员/成员重新登录；公开/受限页面、固定历史、提案及原件读取；搜索重建后匿名排除、授权用户召回 |
| 升级/回退 | 用 PR #25 合并提交 e6f50d3 的真实后端另建旧镜像，验证旧版本 API 读取，再升级并验证内嵌前端；schema 0001–0007 一致 |

首次发布尚无已发行的旧镜像，回退基线是已合并的实际后端源码。
该演练覆盖同 schema 兼容窗口；跨 schema 降级须恢复匹配的数据库与原件备份。
同 schema 镜像切换保留现有数据。恢复备份会丢失备份点后的数据库变更及未备份原件；
恢复目标应使用备份中记录的匹配镜像引用/digest。

## 规模测试

环境：GitHub ubuntu-latest Linux amd64；4 vCPU，Intel Xeon Platinum 8370C，15 GiB RAM，
Docker Engine 28.0.4。构建工具 Node 24.14.1 / Go 1.25.0；运行服务 PostgreSQL 18.6、
Meilisearch 1.55.0。容器共享该主机资源。原始硬件/软件输出在 Container and Compose 日志中。

数据库额外生成 100000 文档 Page：80% 公开、10% 受限、10% 隐藏，每篇约 1 KiB 中英文正文，
各一个候选与发布快照；另有真实 REST 生成的 20 版本历史和固定媒体引用。
规模库发布可见性探针之前共 100003 页，之后共 100004 页。主体为合成数据，原件夹具是一张 PNG。

负载为 100 个并发 Go HTTP 客户端，每个连续执行 20 次匿名搜索（q=knowledge、limit=20），
两轮各 2000 次。延迟包含完整响应与解析；first-pass 是重启应用后的首轮，宿主磁盘/OS 缓存未清空。

| 轮次 | p50 ms | p95 ms | p99 ms | 请求/秒 | 错误率 |
| --- | ---: | ---: | ---: | ---: | ---: |
| first-pass | 4753.012 | 5106.336 | 5247.524 | 20.873 | 0 / 2000（0%） |
| warm | 4733.195 | 5087.220 | 5271.491 | 20.955 | 0 / 2000（0%） |

全量规模索引重建耗时 106 秒；恢复小库索引重建耗时 1 秒。
规模库中真实审核/发布后首次检索可见为 415.755 ms，轮询间隔 100 ms；重建后 outbox
pending/oldest_seconds/failures 均为 0。每个返回 ID 对照数据库当前公开且已发布集合，
同时验证受限页面匿名排除及其作者可检索。

另一次 4 vCPU AMD EPYC 9V45 主机运行的 p95 约 2.98 秒、重建 74 秒；
主机不同，不能当作受控性能对比。上述结果只描述这些环境及负载；当前约 5 秒的百并发
p95 需要纳入站点容量判断，尚未设定或承诺生产 SLO。

## 受控发布状态

远端 main PR/CI gate、v* tag 创建限制与 tag 更新/删除保护已核实 active，与仓库规则 JSON 一致。
`release_ready=true` 登记上述候选验收。PR #31 使用 Refs #11，保留 Issue 直到实际发布完成。

PR #31 审查修订 `7fbadce5401ae31926776ed1b98b134b143dd016` 的
[完整 CI](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39550280618) 已通过；
合并提交 `00366414f99e1dbbc9f6df98104d3adf7900eb42` 的
[main 完整 CI](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39551375008) 也已通过。
审查补充了推送后回读标签及拉取实际 registry 产物的验收，避免仅将构建/推送成功当作发行验收。

受控 annotated tag [v0.1.0](https://github.com/epoch-fresh-2026/Wikit/tree/v0.1.0)
指向上述 main 合并提交，tag object 为 `626093acd261123fdcf3f558f7a38e59cfd831ec`。
创建使用现有仅管理员可创建规则，未修改更新/删除禁止规则。
[Release image 39551445557](https://github.com/epoch-fresh-2026/Wikit/actions/runs/39551445557)
已再次运行完整 CI，再将 Linux amd64 镜像推送到 `ghcr.io/epoch-fresh-2026/wikit`，
记录版本标签、sha-提交标签与 registry digest。推送后从 registry 回读两个标签，要求其 digest
均等于构建输出；按该 digest 拉取并在隔离 Compose 中重新运行完整恢复、回退及规模验收，
通过后才登记发布成功。工作流只在 publish job 取得 packages:write，
不会连接生产站点。该运行的 readiness、完整 validation、publish 及发布后实际镜像验收均已成功。

| 发行产物 | 实际值 |
| --- | --- |
| 版本标签 | `ghcr.io/epoch-fresh-2026/wikit:v0.1.0` |
| 提交标签 | `ghcr.io/epoch-fresh-2026/wikit:sha-00366414f99e1dbbc9f6df98104d3adf7900eb42` |
| Registry digest（两标签相同） | `sha256:7d75052981a890810488d17d2232957139049a2c29082bae3815658069a2441a` |
| 拉取平台 | `linux/amd64`，应用 UID/GID `65532:65532` |

固定产物引用：
`ghcr.io/epoch-fresh-2026/wikit@sha256:7d75052981a890810488d17d2232957139049a2c29082bae3815658069a2441a`。
这是 registry 的 OCI index digest，不是本地 Docker image ID。
发布作业回读两标签验证一致，按该 digest 拉取后命名为本地 `wikit:ci`，该引用用于实际应用容器；
另建的 acceptance target 仅作为隔离测试客户端，未替代被验收应用镜像。

实际发行镜像通过空卷初始化、REST 审核/20 次发布/固定原件、备份校验、拒绝损坏备份及非空恢复目标、
旧凭证撤销、索引重建、真实旧后端回退及再升级。规模库 100004 页，索引重建 123 秒，
发布后检索可见 415.178 ms；100 并发两轮各 2000 请求，错误率均为 0%，
first-pass/warm p95 分别为 5037.121 / 5035.848 ms。环境及缓存边界与前述合成基准一致，
不据此承诺生产 SLO。原始证据见发布运行的 Publish GHCR image 日志及摘要。

当前匿名 pull 授权探测失败；CI 使用发布作业凭证实际拉取，不宣称镜像可匿名读取。
部署者需要有该 package 的拉取权限及相应 registry 登录；本流程没有扩大包可见性。
全部执行均在独立 CI Compose project/卷中完成并清理，没有连接或部署任何生产站点。

部署/备份/恢复命令与兼容边界见 [运维说明](operations.md)。
