package nodes

import (
	"fmt"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func applyNodePatch(sess *domain.Session, node string, patch domain.StatePatch) error {
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		return fmt.Errorf("%s: apply state patch: %w: %v", node, graph.ErrPermanent, err)
	}
	return nil
}
