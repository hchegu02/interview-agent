package retriever

import (
	"math"
	"sort"
)

// Fusion 是融合打分接口。
//
// 输入：候选集（每条带 vec_dist / tag_overlap / 难度差），未排序
// 输出：按 final score 降序排好的 Result，长度可能小于输入（去重 / 截断）
//
// 当前只有 LinearFusion 一个实现。Stage 6 可能加：
//   - RRFFusion：多路 rank 融合，特征尺度不齐时用
//   - CrossEncoderFusion：调外部 reranker 模型（如 bge-reranker）
//
// 抽接口是为了"未来 swap 实现无需改 retriever 主流程"。
type Fusion interface {
	Fuse(candidates []Candidate) []Result
}

// LinearFusion 线性加权融合——
// 三路特征都归一化到 [0,1]，加权求和。
//
// 默认权重（针对面试题库场景）：
//   vector_score      0.60   语义相似度
//   topic_score       0.30   tag overlap ratio
//   difficulty_score  0.10   难度匹配度
//
// 为什么 vector 权重最高但不绝对优势：
//   - 实测下来 vector 召回最稳，主导排序合理
//   - tag 0.30 足以让"话题完全跑偏"的题目被压下去
//   - difficulty 是 tie-breaker，不该主导
type LinearFusion struct {
	WeightVector     float64
	WeightTopic      float64
	WeightDifficulty float64
}

// NewLinearFusion 用合理默认值构造。传 0 走默认。
func NewLinearFusion(wVec, wTopic, wDiff float64) *LinearFusion {
	if wVec == 0 && wTopic == 0 && wDiff == 0 {
		wVec, wTopic, wDiff = 0.60, 0.30, 0.10
	}
	return &LinearFusion{wVec, wTopic, wDiff}
}

func (f *LinearFusion) Fuse(candidates []Candidate) []Result {
	results := make([]Result, 0, len(candidates))
	for _, c := range candidates {
		// vector_score = 1 - cosine_distance，clamp 到 [0,1]
		// pgvector 的 cosine distance 理论范围 [0,2]，正常文本一般 [0,1] 之间
		vs := 1.0 - c.VecDist
		vs = clamp01(vs)

		// tag_score = intersection / query_tag_count
		// query 没传 tag 时（QueryTagCount=0）固定 0 分，不影响纯 vector 检索
		var ts float64
		if c.QueryTagCount > 0 {
			ts = float64(c.TagOverlap) / float64(c.QueryTagCount)
		}
		ts = clamp01(ts)

		// difficulty_score = 1 - |diff - target| / 2
		// 跨度用 2 是因为软过滤就是 ±2，超出范围已经被 SQL filter 掉了
		// 此处分母 2 让 ±1 时得 0.5、±2 时得 0、=target 时得 1
		ds := 1.0 - math.Abs(float64(c.Difficulty-c.TargetDiff))/2.0
		ds = clamp01(ds)

		score := f.WeightVector*vs + f.WeightTopic*ts + f.WeightDifficulty*ds

		results = append(results, Result{
			ID:              c.ID,
			Content:         c.Content,
			Tags:            c.Tags,
			Difficulty:      c.Difficulty,
			Category:        c.Category,
			Score:           score,
			VectorScore:     vs,
			TagScore:        ts,
			DifficultyScore: ds,
		})
	}

	// 按 score 降序排。Score 相同时按 ID 排，保证稳定可复现（单测友好）
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	return results
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
