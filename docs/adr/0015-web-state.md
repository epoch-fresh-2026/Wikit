# 0015 WebUI 状态与请求边界

状态：accepted；日期：2026-10-20。

采用 React/TypeScript/Vite/shadcn/ui，功能模块调用统一同源 REST 客户端，类型从 OpenAPI
生成。不引入第二套权限引擎、离线响应缓存、持久草稿或 Service Worker。

Cookie 保持 HttpOnly，只有不具备独立认证能力的 CSRF nonce 放在标签页 sessionStorage；
个人 Token 和密码不持久化。认证变化卸载所有内容视图和草稿；读取取消并忽略过期响应。
这优先于跨账户保留编辑体验，避免受限内容残留。

写入保留读取时的版本条件及失败请求的幂等 key；冲突由用户明确解决，不自动覆盖。
UI 使用稳定业务错误，不将服务端或网络底层诊断直接呈现。服务端 Core 是授权与状态的唯一裁决者。

Vite 开发代理保留 Origin，后端站点 origin 配为前端 origin；生产静态资源嵌入和容器遵循[部署决策](0016-container-recovery.md)。
