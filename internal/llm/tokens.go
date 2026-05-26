package llm

import "sync/atomic"

// TokenTracker 累计 LLM 调用的 token 用量。
//
// 在哪里挂：
//   - 节点函数 / CallWithSchema 拿到 *Response 后调 Add(resp)
//   - Stage 4 把它注入 Graph callback——OnNodeEnd 时把当前节点累计 token 落库
//
// 为什么用 atomic 而不是 mutex：
//   - 只有 +=，没有读写复合操作
//   - 并发节点（parse_jd 和 parse_resume 同时跑 LLM）都会调 Add，
//     原子操作单条指令搞定，比 mutex 快
type TokenTracker struct {
	prompt     atomic.Int64
	completion atomic.Int64
	calls      atomic.Int64
}

// Add 把一次 Response 的用量累加。response 为 nil 时静默忽略，
// 让调用方无需 nil-check。
func (t *TokenTracker) Add(resp *Response) {
	if resp == nil {
		return
	}
	t.prompt.Add(int64(resp.PromptTokens))
	t.completion.Add(int64(resp.CompletionTokens))
	t.calls.Add(1)
}

func (t *TokenTracker) PromptTokens() int64     { return t.prompt.Load() }
func (t *TokenTracker) CompletionTokens() int64 { return t.completion.Load() }
func (t *TokenTracker) TotalTokens() int64      { return t.prompt.Load() + t.completion.Load() }
func (t *TokenTracker) Calls() int64            { return t.calls.Load() }

// Reset 清零。测试和单 Session 复用时用。
func (t *TokenTracker) Reset() {
	t.prompt.Store(0)
	t.completion.Store(0)
	t.calls.Store(0)
}
