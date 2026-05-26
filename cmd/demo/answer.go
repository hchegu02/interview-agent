// cmd/demo 把候选人回答填到 Session 的辅助函数。
//
// 复制自 internal/httpapi/interview.go:fillPendingAnswer，避免 cmd/demo 反向
// 依赖 httpapi 包（httpapi 引 cmd/demo 没问题，但反过来会把 HTTP 整套拉进
// CLI 二进制，没必要）。
// 语义保持一致：pick_next 暂停 → 写 round.Answer；
//             probe_ask 暂停 → 写 round.FollowUps[last].Answer。
package main

import (
	"fmt"

	"interview-agent/internal/domain"
)

// fillPendingAnswer 把候选人 answer 填到当前 pending 槽位。
// 与 httpapi.fillPendingAnswer 镜像，逻辑须保持同步——节点名常量定义在
// internal/nodes 但用字符串字面量比 import 包级常量更轻。
func fillPendingAnswer(sess *domain.Session, answer string) error {
	switch sess.CurrentNode {
	case "pick_next":
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("no current round for answer")
		}
		round.Answer = answer
		return nil
	case "probe_ask":
		round := sess.CurrentRound()
		if round == nil || len(round.FollowUps) == 0 {
			return fmt.Errorf("no current follow-up for answer")
		}
		round.FollowUps[len(round.FollowUps)-1].Answer = answer
		return nil
	default:
		return fmt.Errorf("session %q is not waiting for answer at node %q", sess.ID, sess.CurrentNode)
	}
}
