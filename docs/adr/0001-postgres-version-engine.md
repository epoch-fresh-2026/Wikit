# PostgreSQL 作为版本与协作权威

采用不可变发布版本和提案快照，不使用 Git 作为运行依赖。产品需要 Git 式协作，
不需要 clone/push；将权限、审核、head、审计与 outbox 放在同一数据库事务，
避免 Git refs 与数据库状态的双重提交问题。

每页 Git 仓库增加小仓库生命周期及备份负担；全站仓库无法在原生 Git 读取下
可靠隔离不同页面权限。代价是三方合并、diff 和撤销语义由应用负责，
并需专项测试。[Git namespace 安全说明](https://git-scm.com/docs/gitnamespaces#_security)。
