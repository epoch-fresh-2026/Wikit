# 0030 子句绑定的检索需求

状态：accepted；日期：2026-10-25。

ADR 0029 的属性列表不能表达“A的X与B的Y”，交叉覆盖会额外要求A/Y和B/X。
新增 `plan.bindings`（最多64项），每项包含subject、attribute、stage、filters。
它是命名主体请求的实际需求；requirements及旧单属性字段保留为兼容摘要。
共享表达“A和B的X、Y”仍展开交叉需求；明确独立子句只展开各自的主体组与属性组。

Core先保护动态主体别名，再沿并列连接词/标点扫描：已有属性的分组遇到新显式主体时开启新组。
同一组的无主体属性续接前面的主体组。使用已有解析器解析每组，保留原文中的阶段、条件与未知词，
不通过固定问句、干员清单或答案映射绑定。组内主体必须与原文主体槽一致；无法确认的归属继续
保留attribute_scope_unverified。全句前置阶段的作用域不明时不猜测。
各子句阶段独立携带；只有局部重解析能够证明的全局multiple_stages/归属疑义才解除。
引用代词、省略导致的歧义及未知表达不得丢弃条件后宣称complete。

明确绑定的需求驱动主体+属性召回、最终授权后的段落选择和逐项覆盖；每个主体只检查实际绑定项。
coverage.requirements新增可空stage，区分同一主体同一属性的不同阶段。
每个绑定独立检查返回证据的页、修订、属性、阶段和filters；最终complete仍要求全部绑定覆盖。
保留4路召回、每页3段、总16段、8主体/8属性/64绑定预算以及当前授权复核。
纯词法路径不变，MCP适配器不复制业务规则，不添加模型或向量依赖。

参考：LlamaIndex的[子问题查询](https://developers.llamaindex.ai/python/examples/query_engine/sub_question_query_engine/)
按数据源拆解问题；[查询转换范例](https://developers.llamaindex.ai/python/examples/query_transformations/query_transform_cookbook/)
将子问题与目标工具配对。Haystack的[问题分解范例](https://haystack.deepset.ai/blog/query-decomposition)
保留question/context对应关系。借鉴其显式绑定与独立取证结构，具体实现适配Wikit的确定性全文检索边界。
本轮Context7请求因月额度耗尽失败，以上官方页面为回退参考。

验收：任意名称、共享组、独立组、属性续接、局部阶段/filters、未知与受限主体、预算和错误归属负例。
现有594道PRTS（原514+40+40）及24道合成题全部回归，共618题；原评分和原题保持不变。
再冻结实现与评测器，然后编写全新holdout18、冻结题集后首次执行，全部失败保留且不据新题调规则。
