# 0028 RAG 排序与证据预算

状态：accepted；日期：2026-10-25；关联 Issue #62。用户授权参考开源 RAG 优化链路并自建数据集验证。

沿用 MCP-only、最多四路全文查询、最终授权事务和无向量/站内模型边界。
参考 LlamaIndex QueryFusionRetriever 的 reciprocal rank fusion：按 page/revision
累计 `1/(60+rank)`（rank 从 1 开始），每个分支同一候选只投一次票，同分按首次出现顺序。
不同 revision 不相互加分；最终只使用与当前 head 相同且当前可读的候选。
最终事务过滤后，对 named 问题先按融合顺序给每个主体一个页面名额，再填余下页面，
避免大量同名页面挤掉另一比较主体。此策略不改变主体解析规则。

RAG 段落在截取前三条之前，仅对同一页面、同一章节中完整相同的文本去重。
不同章节、不同页面以及截断文本不去重，避免丢失语义环境、独立来源和未显示内容。
保留第一条的源行号和固定 revision 引用。普通搜索的段落行为保持原样。
Haystack SentenceWindowRetriever 提示上下文必须保留来源；本轮不直接拼接邻接段落，
因为现有契约要求每条引用精确对应源块，盲目扩窗会模糊证据与覆盖边界。

证据仍按页面轮询，最多16条/每条600字符；若独立段落被16条预算丢弃，
coverage.truncated 必须为 true。不以页面已召回代替返回证据完整。

先冻结自建合成文档和问题、判分逻辑，再用旧/新实现运行同一真实 MCP 集成评测。
覆盖重复段落、同名主体、章节隔离、长文本、预算和无答案；记录证据召回及完整命中，
不将合成集成绩或检索指标当作线上泛化效果/生成准确率。既有问句和权限竞态测试继续回归。

参考（2026-10-25访问）：
- https://github.com/run-llama/llama_index/blob/main/llama-index-core/llama_index/core/retrievers/fusion_retriever.py
- https://docs.haystack.deepset.ai/docs/3.1/sentencewindowretriever
- https://github.com/stanford-futuredata/ARES （检索相关性、答案忠实性与答案相关性分开评价）
