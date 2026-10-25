# 0027 结构化问题与证据覆盖

状态：accepted；日期：2026-10-24；关联 Issue #63。

新增40题验证表明句尾模板无法可靠处理口语及比较，存在把局部资料当完整证据的风险。
MCP rag 改为独立提取主体、属性、阶段和比较条件；属性同义词与通用语法可以扩展，
不得包含问题ID、完整测试问句、干员名单或答案映射。动态主体别名仅来自已授权候选标题，
未知主体保留。最多四次全文查询，包括属性发现与最多三条主体查询；集合问题按发现结果取证。

新增 plan 与 coverage 契约。coverage.status 为 complete/partial/insufficient/unverified，
记录逐主体的 covered/not_found/attribute_missing/stage_unsupported/unverified、页面ID、
missing_subjects、unsupported_constraints、truncated。无权读取和不存在统一 not_found，
不返回任何早先候选的受限元数据。所有状态在聚合后的最终授权事务内基于当前证据重新判断。

阶段支持须能把源块阶段列与属性值对应；占位值或不存在的阶段不能借用其他列。
不能可靠关联时标记 unverified。证据预算或候选截断不得宣称 complete。
rag.status 增加 partial_evidence；有相关段落也可能为 insufficient_evidence，
此时只能解释缺失/不支持，不能生成所问数值。默认 lexical、REST、Web 不变，无向量或外部模型。

原40题从独立测试集转为回归集；修复冻结后新增问题先冻结再实际MCP验收，报告所有失败。
检索/覆盖指标不等于模型生成正确率。
