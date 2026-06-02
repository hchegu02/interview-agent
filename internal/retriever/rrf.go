package retriever

import "sort"

// MergeRRF uses reciprocal rank fusion to merge ranked candidates from multiple
// retrieval stages. The returned Result.Score is the RRF ordering score; do not
// compare it directly with LinearFusion's 0..1 normalized score.
func MergeRRF(stageResults [][]StageResult, k int, rankConstant float64) []Result {
	if k <= 0 {
		k = 5
	}
	if rankConstant <= 0 {
		rankConstant = 60
	}
	type merged struct {
		result Result
		score  float64
	}
	byID := map[string]merged{}
	for _, stage := range stageResults {
		for i, item := range stage {
			id := item.Result.ID
			if id == "" {
				continue
			}
			rank := item.Rank
			if rank <= 0 {
				rank = i + 1
			}
			score := 1 / (rankConstant + float64(rank))
			current := byID[id]
			if _, ok := byID[id]; !ok {
				current.result = item.Result
			}
			current.score += score
			byID[id] = current
		}
	}
	out := make([]Result, 0, len(byID))
	for _, item := range byID {
		item.result.Score = item.score
		out = append(out, item.result)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}
