## ADDED Requirements

### Requirement: 题库 JSON 导入 contract 必须版本化且稳定可验证

系统 MUST 明确定义本地题库 JSON 导入的版本化输入契约、字段兼容规则和错误语义，并用自动化测试覆盖该 contract。

#### Scenario: 接受版本化导入包

- **WHEN** 用户导入包含 `schema_version`、`source_ref`、`items`、`validation_report` 或 `review_policy` 的题库导入包
- **THEN** 系统 MUST 按声明版本解析导入包
- **AND** 系统 MUST 保留 source、validation 和 review policy 相关事实用于暂存 review 和后续诊断
- **AND** 系统 MUST NOT 因额外 contract 元数据破坏正式 `Item` 写入格式

#### Scenario: 接受标准数组和 wrapped items

- **WHEN** 用户导入 JSON 题库文件
- **AND** JSON 顶层是题目数组或 `{ "items": [...] }`
- **THEN** 系统 MUST 将 legacy 输入按当前导入包 contract 语义归一化
- **AND** 后续 normalize、暂存、review 和 commit 流程 MUST 不依赖顶层包装差异

#### Scenario: 兼容真实 LLM 输出的标量漂移

- **WHEN** 导入 JSON 中 `difficulty` 是数字字符串
- **AND** `rubric` 是 object、string array 或 string
- **AND** `tags`、`expected_points`、`role_tags` 或 `follow_up_hints` 是 string array 或分隔字符串
- **THEN** 系统 MUST 将这些字段归一化为正式 `Item` 字段类型
- **AND** 分隔字符串 MUST 至少支持英文逗号、英文分号、竖线、中文逗号、中文分号和顿号

#### Scenario: 不支持的字段类型必须报错

- **WHEN** JSON 字段存在
- **AND** 该字段不是 contract 允许的类型
- **THEN** 系统 MUST 拒绝本次解析
- **AND** 错误信息 MUST 包含出错字段名
- **AND** 错误信息 SHOULD 包含字段路径和原始值摘要
- **AND** 系统 MUST NOT 将坏字段静默转换为空值

#### Scenario: contract 变化必须有 golden 或回归测试

- **WHEN** 题库 JSON 导入 contract 新增兼容格式、错误语义或字段转换规则
- **THEN** 系统 MUST 增加或更新对应自动化测试
- **AND** 测试 MUST 覆盖成功归一化和失败报错两类路径

### Requirement: Review 和 commit 边界必须形成发布事务

系统 MUST 保持导入解析、暂存 review 和正式 commit 的职责边界，并将 commit 作为可诊断发布事务处理。任何 JSON 输入、LLM 输出、脚本产物或来源适配器都不得直接写入正式题库。

#### Scenario: 导入解析只产生暂存项

- **WHEN** 用户导入本地题库 JSON 或源文档生成题
- **THEN** 系统 MUST 先创建 import job 和暂存 import items
- **AND** 系统 MUST NOT 在解析阶段写入正式 `question_bank`

#### Scenario: commit 只写入满足门禁的题目

- **WHEN** 用户提交 import job
- **THEN** 系统 MUST 只写入 valid、人工 accepted、Agent review 非阻塞、非重复且质量门禁通过的题目
- **AND** 系统 MUST 跳过 rejected、needs_human_review、重复或高风险脏题
- **AND** 被跳过题目 SHOULD 保留可诊断原因

#### Scenario: commit summary 记录发布事务结果

- **WHEN** commit 完成、部分完成或失败
- **THEN** 系统 MUST 返回或持久化 commit summary
- **AND** summary MUST 至少区分 matched、imported、skipped、embedding synced、embedding failed 和 failure reasons
- **AND** summary MUST 能支持维护者判断是否需要重试、reindex 或人工处理

#### Scenario: embedding 失败不得静默分叉

- **WHEN** 题目已写入正式 `question_bank`
- **AND** embedding 写入或维度校验失败
- **THEN** 系统 MUST 保留正式题库事实
- **AND** 系统 MUST 标记 embedding 状态和错误原因
- **AND** 系统 MUST 提供后续 reindex 或 retry 的事实入口

#### Scenario: 来源适配器不得直接发布

- **WHEN** 系统通过脚本、Skill、MCP、外部链接或本地文档获得题库来源
- **THEN** 该来源 MUST 进入现有导入暂存流程
- **AND** 适配器 MUST NOT 直接写入正式 `question_bank`
