# 0017 契约生成器与应用编译器分离

状态：accepted；日期：2026-10-21。

Web 应用升级至 TypeScript 7.0.2；openapi-typescript 7.13.0 的 peer dependency
仍要求 TypeScript ^5.x，且类型生成依赖其编译器 API。禁止使用 force、legacy-peer-deps
或放宽检查绕过不兼容。

将 openapi-typescript 7.13.0 与 TypeScript 5.9.3 放在根目录契约工具链，web 只保留
TypeScript 7.0.2。web 的生成和 check:api 脚本显式调用根目录生成器；每个 CI job
和 Docker 前端构建都先安装根目录工具，再安装 Web 依赖。生成声明仍由 TypeScript 7
进行应用类型检查，生成内容不得漂移。开发安装需同时执行两处 npm ci。

Node 运行基线继续为 24；本决定不改变 API 契约、应用权限或产品范围。
