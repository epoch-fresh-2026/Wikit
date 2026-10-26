# 0014 MCP 传输与 Core 等价性

状态：accepted；日期：2026-10-20。

使用官方 Go SDK v1.7.0，机器工具目录嵌入程序，由 MCP 适配器直接分派到既有 Core。
不回环调用 REST，不新增凭证管理、OAuth、resources、prompts 或模型调用能力。
session 绑定账户及凭证代次；HTTP 入口逐请求鉴权，工具事务仍以当前 Token 重新鉴权。
Origin 与固定站点 Host 检查由外层完成，SDK 处理版本、JSON-RPC 与 session 协议。

输入 schema 完整编译并对外公布；传输只校验结构，业务值约束留在 Core，以保持 REST
错误语义。输出完整验证，业务错误使用共享的稳定错误 DTO，不输出内部诊断。

REST 与 MCP 共用 JSON body 摘要，忽略对象字段顺序及无意义空白，不改变字符串或数字词法。
摘要携带旧原始字节摘要作为兼容分量，原有记录仍接受原 body 表示的重试；不迁移正文或保存原请求。
幂等结果和游标继续由 Core 重新鉴权；session 本身不能授予对象权限。
