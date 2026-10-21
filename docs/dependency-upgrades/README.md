# 依赖升级集成（Issue #33）

日期：2026-10-21；用户已确认升级全部十项依赖。

原 PR #12、#13、#14、#15、#16、#26、#27、#28、#29、#30 先 squash 合入维护集成分支，
最终集成 PR 必须基于最新 main 通过全部启用的 CI 后合并。中间分支只组装候选，
不代表生产发布或主分支验收完成。main 的保护规则和所有 CI 阶段保持启用。

| 依赖 | 目标版本 |
| --- | --- |
| TypeScript（应用编译器） | 7.0.2 |
| React Router | 8.4.0 |
| jsdom | 30.1.3 |
| @types/node | 26.6.5 |
| Prettier | 3.9.10 |
| actions/setup-go | 7.0.0 |
| actions/checkout | 7.0.1 |
| actions/setup-node | 7.1.0 |
| docker/login-action | 4.6.0 |
| docker/build-push-action | 7.4.0 |

运行环境继续使用 Node 24、Go 1.25。jsdom 30 要求最低 Node 24.15.0，Docker 与 engines
同步提升；@types/node 26 不意味着允许使用 Node 26 专有 API。
openapi-typescript 7.13.0 仍要求 TypeScript ^5.x，需要将生成器与应用编译器隔离。

每项兼容性处理见本目录的 PR 记录。最终 CI 包括文档/契约、Go vet/race、API 类型同步、
strict、组件测试、生产构建、真实 PostgreSQL/Meilisearch 集成，以及完整容器恢复/规模验收。
发布 Actions 另核对固定 SHA、输入、权限和输出；未实际执行的 GHCR 推送单独记录，
不为依赖维护而创建新的发布标签。

## 本地组合验证

Node 24.15.0（官方可执行文件且 SHA-256 与发布清单一致）下，根目录和 web 干净 npm ci、
API 类型同步、TypeScript 7 strict、28 项组件测试、生产构建全部通过；Prettier CLI 报告 3.9.10。
Go 源码未改变，本次已运行 gofmt、go vet 与 go test -race -count=1 ./...，全部通过。
本机未安装 Docker，最终 PostgreSQL/Meilisearch 集成、容器恢复和规模验收以集成 PR 的完整 CI 为准。
组装期间的中间 PR 运行不作为最终验收证据；重复运行可在候选组装完成后取消。
