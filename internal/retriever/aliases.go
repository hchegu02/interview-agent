package retriever

import "strings"

// canonicalTags 是 tag 同义词 → 规范化 tag 的映射。
//
// 为什么放 Go 内存而不是 PG 表：
//   - Stage 2.4 规模小（~20 条），硬编码改起来比改 SQL 快
//   - 让 retriever 包"自描述"——读代码就能知道有哪些同义词
//   - 上规模（>100 条 / 跨团队维护）时迁到 PG 是一个 migration 的事，
//     CanonicalizeTags 函数签名不会变
//
// 规范化的"canonical"命名约定：domain_subtopic
//   - go_concurrency / go_runtime / go_stdlib
//   - redis_persistence / redis_ha / redis_datastructure
//   - pg_internals / pg_index / pg_replication
//   - system_design / distributed
//
// 注意：seed 数据里的 tags 应该直接写 canonical 名，避免每次入库都做归一化；
// 这个表只用于"用户查询时输入的口语化 tag → canonical"这条路径。
var canonicalTags = map[string]string{
	// Go 并发
	"channel":     "go_concurrency",
	"goroutine":   "go_concurrency",
	"mutex":       "go_concurrency",
	"sync":        "go_concurrency",
	"gmp":         "go_concurrency",
	"scheduler":   "go_concurrency",

	// Go runtime
	"gc":          "go_runtime",
	"gctrace":     "go_runtime",
	"escape":      "go_runtime",
	"pprof":       "go_runtime",

	// Redis 持久化
	"aof":         "redis_persistence",
	"rdb":         "redis_persistence",
	"snapshot":    "redis_persistence",
	"fsync":       "redis_persistence",

	// Redis 高可用
	"sentinel":    "redis_ha",
	"cluster":     "redis_ha",
	"failover":    "redis_ha",
	"replication": "redis_ha",

	// Redis 数据结构
	"ziplist":     "redis_datastructure",
	"skiplist":    "redis_datastructure",
	"hash_table":  "redis_datastructure",

	// PG 内核
	"mvcc":        "pg_concurrency",
	"vacuum":      "pg_internals",
	"wal":         "pg_internals",
	"hot":         "pg_internals",

	// 系统设计
	"qps":         "system_design",
	"cap":         "system_design",
	"sharding":    "distributed",
	"consensus":   "distributed",
	"raft":        "distributed",

	// 产品名 / 技术栈同义词归一化
	// 简历和 JD 里这些写法极其常见,统一拉到 canonical 短名,
	// 便于 gap_analyze 集合运算和 RAG tag 匹配。
	"golang":     "go",
	"postgresql": "pg",
	"postgres":   "pg",
	"nodejs":     "node",
	"node.js":    "node",
	"k8s":        "kubernetes",
}

// CanonicalizeTags 把口语化 tags 翻译成规范化形式。
//
// 规则：
//   - 全部 lowercase 后查表，命中则替换；未命中保留原样（让 GIN 索引仍可能匹配）
//   - 去重并保持稳定顺序（PG 的 tags && ARRAY[...] 对顺序不敏感，
//     但稳定输出方便单测断言）
//   - nil / 空切片 → 空切片
//
// 例子：
//   ["Channel", "AOF", "QPS"] → ["go_concurrency", "redis_persistence", "system_design"]
//
// 故意不接 stemmer / 中文分词：seed 都用英文 + canonical tag，
// 用户层即便输入中文（如"通道"）也会走到 vector 检索那条路，
// tag 只做硬性话题约束，不追求 NLP-style 召回。
func CanonicalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if c, ok := canonicalTags[t]; ok {
			t = c
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
