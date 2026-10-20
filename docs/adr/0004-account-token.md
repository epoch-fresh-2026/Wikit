# 每账户一个有效 Token

Agent 通过用户 Token 使用同一账户身份，不建立 Agent 专有角色或多凭证模型。
这符合简单接入目标，但多个 Agent 共用 Token 时不能可靠区分具体操作者；
轮换会同时使所有使用旧 Token 的客户端失效。

Token 只保存摘要，可撤销或轮换。MCP 首版限于支持预配置 Bearer Token 的
客户端，不宣称实现完整 OAuth 授权发现流程。
