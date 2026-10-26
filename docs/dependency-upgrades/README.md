# 依赖维护与兼容性

依赖版本以清单与锁文件为准，第三方 Actions 固定完整 SHA。依赖维护不改变产品版本 `0.1.0`。
运行环境为 Node 24.15.0 起且低于 25、Go 1.25；构建期类型定义和 Action 自身运行时不改变应用运行基线。

## 工具链边界

Web 使用 TypeScript 7.0.2，契约生成使用 openapi-typescript 7.13.0 与 TypeScript 5.9.3，
二者隔离在 Web 与根目录工具链，原因见 [ADR 0017](../adr/0017-contract-generator-toolchain.md)。
路径别名使用相对配置文件位置的 `@/*` → `./src/*`，与 Vite 一致，不使用已删除的 baseUrl。
React Router 8.4.0 保持 BrowserRouter 与 declarative Routes/Route，不引入 framework mode。
jsdom 30.1.3 要求最低 Node 24.15.0，engines 与 Docker 构建环境使用同一边界。
@types/node 26.6.5 仅提供构建期类型，不能据此使用 Node 26 专有 API。
Prettier 3.9.10 负责格式化，升级不要求批量改写无关源码。

## 工作流依赖

| Action | 版本 | 保留的约束 |
| --- | --- | --- |
| actions/setup-go | 7.0.0 | 应用 Go 版本来自 go.mod，核对缓存与版本输入 |
| actions/checkout | 7.0.1 | 需要历史的任务使用 fetch-depth: 0，persist-credentials: false；不启用不安全 PR checkout |
| actions/setup-node | 7.1.0 | Node 24 与 npm 缓存，应用仍受 engines 最低版本约束 |
| docker/login-action | 4.6.0 | 核对 registry/username/password；仅发布任务以 GITHUB_TOKEN 登录 GHCR |
| docker/build-push-action | 7.4.0 | 核对 context/push/platforms/labels/tags 与 digest 输出，发布 linux/amd64 版本和 SHA 标签 |

PR 验证使用只读权限；仅发布任务拥有 packages: write，不使用 pull_request_target 执行贡献者代码。
升级后核对固定 SHA、输入、权限和输出，不为验证依赖维护创建发布标签。

## 验证与证据

每次维护均须通过文档与契约检查、Go vet/race、API 类型同步、Web strict、组件测试、生产构建、
真实 PostgreSQL/Meilisearch 集成与容器恢复和规模验收。检查入口见[开发工作流](../development.md)。
中间集成分支不代表主分支验收完成；实际 GHCR 推送与镜像运行单独登记。

2026-10-21 的组合验证在校验官方 SHA-256 的 Node 24.15.0 下完成根目录与 Web 干净安装，
API 类型同步、TypeScript strict、28 项组件测试、生产构建及 Prettier 版本检查通过；
Go 格式、go vet 与 go test -race -count=1 ./... 通过。
此前 Node 24.14.1 下的生成器迁移预检通过，但该版本不满足最终 jsdom 运行基线。
当时本机没有 Docker，未在本机验证容器；完整发布运行证据见[发布验收记录](../release-readiness.md)。
