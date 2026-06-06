package nodes

// promptParseJD 是 parse_jd 节点用的 system prompt。
//
// 设计：
//   - 用"字段+类型+一句话说明"的列表给 schema，比塞 JSON Schema 标准格式有效
//   - 强调"只返回 JSON，不要 markdown"——配合 ResponseFormat=json_object 双重约束
//   - level 给 enum 限定，避免 LLM 写 "高级"、"middle" 这种自由值
//   - years_required 找不到时返回 0，明确语义（vs 让 LLM 编一个）
const promptParseJD = `你是岗位 JD 分析助手。从下面的 JD 文本中抽取结构化信息，**只返回 JSON 对象**，不要任何解释或 markdown。

输出 schema（字段都必填，找不到就给空值）：
- title          string         岗位名称（如 "Go 后端开发工程师"）
- level          string         enum: intern | junior | senior | staff（找不到就给 "junior"）
- key_skills     []string       必备技能短语，归一化小写英文/拼音（如 "go", "redis", "mysql"）
- must_have      []string       JD 中明确写"必须"、"硬性要求"的条目；普通"熟悉/掌握"不算
- nice_to_have   []string       加分项
- years_required int            JD 提及的最低年限（如 "3 年以上" → 3）；找不到就 0

要点：
1. key_skills 与 must_have 的并集应覆盖全部技能项；must_have 是 key_skills 的子集
2. 同义技能合并：Golang/Go 统一 "go"；MySQL/mysql 统一 "mysql"
3. 不要输出多余字段；不要包含 JD 原文

JD 文本：
%s
`

// promptParseResume 是 parse_resume 节点用的 system prompt。
//
// 关键决策：
//   - highlights 是 dynamic probing 节点的输入,所以这里要让 LLM 抽"可被追问的点"
//     而不是"自吹自擂的形容词"——例如 "实现了一个秒杀系统支持 10w QPS" 是可追问的,
//     "代码质量好" 不是
//   - projects.stack 字段供后续 RAG query 增强用
const promptParseResume = `你是简历分析助手。从下面的简历文本中抽取结构化信息，**只返回 JSON 对象**，不要任何解释或 markdown。

输出 schema（字段都必填，找不到就给空数组/0）：
- years      int               候选人累计相关工作年限（按毕业时间或第一份相关工作起算）
- skills     []string          掌握的技能短语，归一化小写英文（如 "go", "redis", "kafka"）
- projects   []object          项目经历列表，每项含：
    - name        string       项目名
    - role        string       角色（如 "后端主力", "核心开发"）
    - highlights  []string     2-4 条"可被技术追问的具体动作或数据"，例如
                              "用 Redis lua 实现库存预扣支撑 1w QPS"
                              **避免** 形容词式描述如 "代码质量好"、"性能优秀"
    - stack       []string     技术栈关键词
- highlights []string          从所有项目里提炼的 3-6 条"全简历最值得追问的点"

要点：
1. highlights 必须是事实陈述+量化指标/技术细节，不要主观评价
2. 同义技能合并（Golang→go）
3. 不要输出原文，不要输出多余字段

简历文本：
%s
`

// promptGapStrategyFallback 是 gap_analyze 节点在边界场景兜底用的 prompt。
//
// 触发条件：OverlapScore ∈ [0.3, 0.7) 这种规则法不灵的中间地带。
// 强匹配（>=0.7）直接 validate，弱匹配（<0.3）直接 cover_gap，无需 LLM。
const promptGapStrategyFallback = `你是面试官助手。根据以下匹配情况，决定面试提问倾向。

匹配技能：%v
缺失技能：%v
重叠分数：%.2f
候选人年限：%d，岗位要求年限：%d

返回 JSON：
- strategy  enum: validate | explore | cover_gap
    validate:  深度题为主，验证简历真实性（高匹配适用）
    explore:   优先 probe 重叠技能 + 追问项目细节（中匹配适用）
    cover_gap: 用题库题覆盖 missing skills、定位水平（低匹配适用）
- reason    string，30 字以内的判断依据，给报告页透出

只返回 JSON 对象。
`

// promptPickNext 是 pick_next 节点的 system prompt。
//
// 设计：
//   - 给 LLM 看的内容: 候选题列表(已过滤掉问过的) + WorkingMemory 概况 + GapStrategy
//   - 让 LLM 同时输出 next_question_id + 一句话 reasoning,后者给报告页的"为什么问这道"展示
//   - 强调"必须从候选列表里选 id",代码侧再做一次硬校验,LLM 编 ID 会被 ErrSchemaInvalid 拦下
//   - skill_coverage 用 map[string]int 直接 marshal,LLM 看一眼就知道哪个技能问太多了
const promptPickNext = `你是技术面试官。从候选题库中挑下一道题。**只返回 JSON 对象**,不要解释。

岗位:%s    候选人年限:%d / 要求:%d
GapStrategy:%s    Reason:%s
确认掌握技能:%v
薄弱技能:%v
已被验证次数(skill_coverage):%s
当前动态难度:%s    目标题目难度:%d
近三轮平均分:%.1f    已问轮次:%d / %d
近三轮回顾(按时间顺序):
%s

候选题(必须从下面 id 中选一个):
%s

输出 schema:
- next_question_id  string  候选列表里的 id,严格匹配
- reasoning         string  30 字以内,说"为什么是这道"——结合 strategy/coverage/score

选题原则:
1. validate 倾向: 选难度更高、考点和 confirmed_skills 相关的题
2. cover_gap 倾向: 选基础题,考点覆盖 missing_skills
3. explore 倾向: 选 coverage 计数低的技能,做面的探索
4. 动态难度: 优先选择接近目标题目难度的题,同等难度距离下再看 coverage
5. 已问过的题不会出现在候选里,不必担心重复
`

// promptEvaluate 是 evaluate 节点的 system prompt。
//
// 设计:
//   - 输入 = Question + ExpectedPoints + Answer; 让 LLM 对照要点给分,
//     而不是凭印象拍 0-100。
//   - ExpectedPoints 为空时 prompt 显式说"无参考要点,按一般认知给分",
//     避免 LLM 因为缺要点就拒答或乱给。
//   - score 强制 0-100; strengths/weaknesses 数组要短(各 1-3 条);
//     suggestion 一句话面试官式反馈。
const promptEvaluate = `你是面试官,评估候选人对一道技术题的回答。**只返回 JSON 对象**,不要任何解释或 markdown。

题目:%s

期望要点(参考标准,可空):
%s

候选人回答:
%s

输出 schema(字段都必填):
- question_id   string         题目 ID:"%s"
- score         int            0-100:覆盖 80%%+ 要点且表述清晰 ≥ 80;答到一半 50-70;答非所问/严重错误 < 30
- strengths     []string       1-3 条候选人答得好的具体点(对照要点或答案中的亮点)
- weaknesses    []string       1-3 条没答到或答错的点
- suggestion    string         一句面试官反馈,30 字内,告诉候选人下次怎么答更好

要点:
1. 必须基于"期望要点"给分;期望要点为空时按通用工程师标准评估
2. score 是整数,不要 92.5 这种小数
3. 不要输出题目原文、不要 markdown
`

// promptCritic 是 critic 节点的 system prompt。
//
// 合并两个判断到一次 LLM 调用:
//  1. evaluation 是否"靠谱"(grounded_score / need_refine / issues)
//  2. 这道题是否值得追问(has_probe_signal / probe_topic)
//
// 两个判断都基于同样的 question + answer + evaluation 上下文,
// 共用 prompt 省 50%% token,且能让 LLM 在同一推理流里互相校准
// (比如"评估准确" + "答案值得追问")。
const promptCritic = `你是面试官的反思助手,审视一次评估并判断是否值得追问。**只返回 JSON 对象**,不要任何解释或 markdown。

题目:%s

候选人回答:
%s

评估系统给出的结论:
- 分数:%d
- 优点:%v
- 不足:%v
- 建议:%s

期望要点(参考):
%s

输出 schema:
- grounded_score   int       0-100,这次评估在多大程度上"基于事实/要点"而非编造
                              (≥80 高质量,60-79 一般,<60 不靠谱)
- need_refine      bool      grounded_score < 60 或评估明显偏离要点时为 true
- issues           []string  指出评估的具体问题(2-4 条);need_refine 为 false 时可空
- summary          string    一句话总结这次评估的可靠性,30 字以内
- has_probe_signal bool      候选人答案中是否有"具体技术细节/亮点"值得深挖
                              提到了 QPS/具体数字/独特做法 → true
                              答得很泛/没具体内容 → false
- probe_topic      string    has_probe_signal=true 时,挑一个最值得追问的具体点
                              (10-30 字,例如 "Redis lua 库存预扣的并发控制")
                              false 时给空串

要点:
1. grounded_score 衡量"评估打分对不对",不是答案本身有多好
2. has_probe_signal 与 need_refine 独立——评估靠谱也能有追问信号
3. 答案为空或评估 score=-1 时,has_probe_signal 一定 false
`

// promptRefine 是 refine 节点的 system prompt。
//
// 设计:
//   - 同样的 evaluate schema(强行让 LLM 输出格式不变,方便单一 validator 复用)
//   - 把 critic 指出的 issues 显式塞进 prompt 让 LLM 知道"为什么要重评"
//   - 同时给出"原评估"作为对照,让 LLM 看到自己上一版的偏差
const promptRefine = `你是面试官,根据 critic 反馈重做一次评估。**只返回 JSON 对象**,不要解释或 markdown。

题目:%s

候选人回答:
%s

期望要点:
%s

原评估(被 critic 指出有问题):
- 分数:%d
- 优点:%v
- 不足:%v
- 建议:%s

Critic 指出的具体问题:
%s

请基于 critic 的反馈重新评估,输出与原评估同 schema:
- question_id   string  "%s"
- score         int     0-100
- strengths     []string  1-3 条
- weaknesses    []string  1-3 条
- suggestion    string  一句话面试官反馈

要点:
1. 不要简单复制原评估;必须针对 critic 的 issues 修正
2. 不要矫枉过正——critic 说评估偏高就大幅压分会失真,理性调整
3. score 必须是整数 0-100
`

// promptProbeAsk 是 probe_ask 节点生成追问问题的 prompt。
//
// 让 LLM 把抽象的 probe_topic 转成具体的、面试官口吻的一句追问。
// 包含主题题 + 候选人主回答 + 已有 followup(多轮 probe 时)给 LLM 上下文。
const promptProbeAsk = `你是技术面试官,正在追问候选人。**只返回 JSON 对象**,不要 markdown。

主题题:%s

候选人主回答:
%s

%s

需要深挖的主题:%s

输出 schema:
- question  string  一句话追问,面试官口吻,15-50 字
                    要具体到能用一句话回答的程度,例如:
                    "你提到用 lua 实现库存预扣,具体怎么处理超卖的边界 case?"
                    避免泛问 "再讲讲" / "展开一下"
- reason    string  10-30 字,说明为什么问这个(回报给报告页透出)

要点:
1. 紧扣 probe_topic 给的方向,不要跑题
2. 不要重复主题题或前面的追问
3. 不要"是不是"这种封闭问句,要开放式问法
`

// promptProbeEval 是 probe_eval 节点的 prompt。
//
// 双输出:评分追答 + 决定是否继续追问。和 critic 一样的合并思想,
// 同一轮上下文里两个判断一起出。
const promptProbeEval = `你是面试官,刚问完一个追问,需要评分并决定是否继续深挖。**只返回 JSON 对象**,不要 markdown。

主题题:%s
追问:%s
候选人对追问的回答:
%s

输出 schema:
- score              int     0-100,追答的质量
                              答到追问点 ≥ 70;答非所问 < 40;空答 0
- strengths          []string  1-2 条追答的亮点
- weaknesses         []string  1-2 条追答的不足
- suggestion         string   一句反馈,30 字内
- has_more_probe     bool     追答里又冒出新的、值得继续深挖的具体点 → true
                              答得很泛 / 已经无新增信息 → false
- next_probe_topic   string   has_more_probe=true 时给一个新主题(10-30 字)
                              false 时空串

要点:
1. score 是整数 0-100
2. has_more_probe 慎用 true——只在追答暴露出新的具体细节(数字/方案名/边界)时才 true
3. 答案为空或纯空白时,score=0 且 has_more_probe=false
`

// promptReflectionCheck 是 reflection_check 节点的 prompt。
//
// 节点是 Agent 循环出口决策点:看 working memory 决定继续出新题 / 反思补漏 / 终止。
// 不评估单道题,只评估"整体面试进度"。
const promptReflectionCheck = `你是面试官,刚结算完一道题,正在决定下一步。**只返回 JSON 对象**,不要 markdown。

面试进度:
- 已问主题题:%d / %d
- 当前平均分:%.1f
- 已确认掌握的技能:%v
- 表现薄弱的技能:%v
- 候选人简历声明但未验证的技能:%v
- 反思补漏预算剩余:%d
- 主题题预算剩余:%d

可选动作:
- ask_new   继续出新题(常态)
- reflect   薄弱技能值得专门补一道题深究
- end       覆盖度够 / 预算耗尽 / 候选人能力已经明确,提前结束

输出 schema:
- action          string   ask_new / reflect / end 三选一
- reasoning       string   20-60 字决策理由
- reflect_topic   string   action=reflect 时给主题(对应一个技能名,如 "redis"),其他情况空串

要点:
1. WeakSkills 非空 + 还有反思预算时,优先考虑 reflect(只用一次的机会要花在刀刃上)
2. 如果还没问够题(主题题预算 > 0) 一般 ask_new,不要轻易 end
3. end 只在能力已经明确(均分两端极端) 或 预算即将耗尽 时给出
4. reflect_topic 必须是 WeakSkills 里出现过的技能名,不要凭空编
`
