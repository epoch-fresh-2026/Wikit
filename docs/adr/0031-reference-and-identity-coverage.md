# 0031 句内指代与主体存在性分离

状态：accepted；日期：2026-10-25；关联 Issue #63，用户确认本轮修复指代与缺属性误报。

在子句绑定前消解同一问题内、明确主体槽中的引用。单数“他/她/它”只绑定前文唯一显式主体；
复数“他们/她们/它们”绑定前文显式主体组；“前者/后者”要求恰好两个有序主体。
不根据名称推断性别，不读取聊天历史，不跨越未来主体。动态名称先保护，避免名称内部字符被改写。
未消解的引用保留 unresolved_reference 约束和 unverified 覆盖，不能当作新主体不存在或声称 complete。
原始 question 保留；规范化后的主体进入既有 bindings、局部阶段/filters 和逐项证据校验。

属性召回未发现某命名主体时，优先在原四路预算内执行主体名查询；该分支仍使用相同授权、类型、
标签过滤和最终当前修订复核。存在性与属性取证分离：授权主体页可以没有所问属性的段落，覆盖为
attribute_missing；未执行主体查找、存在性查询截断或已确认主体被输出预算省略时为 unverified。
not_found 只表示当前受限检索未找到匹配主体，不宣称全库不存在。主体名查询替代该主体的附加
属性查询，页面原文仍用于逐项取证；不改变普通关键词检索或扩展检索/证据预算。

参考 LlamaIndex [Condense Plus Context](https://developers.llamaindex.ai/python/examples/chat_engine/chat_engine_condense_plus_context/)
在检索前转换为独立问题的流程，以及 Haystack [FilterRetriever](https://docs.haystack.deepset.ai/docs/filterretriever)
将文档筛选与相关性评分分离的结构。这里是适配 Wikit 边界的确定性实现，不引入模型依赖，
也不将属性词当作身份确认的必要条件。Context7 配额耗尽，使用官方文档/源码回退。

验收覆盖任意名称、单复数/顺序引用、歧义/无前件/未来前件、名称保护、阶段、缺属性、未知及受限主体、
类型/标签过滤、最终授权和预算。先回归全部658题（634 PRTS和24合成），保留原评分与失败；
冻结实现和评测器后再编写全新holdout19并冻结题集，首次执行后只记录分析，禁止据新题改规则。
