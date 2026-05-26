package nodes

import "interview-agent/internal/domain"

func markDegradedReason(mem *domain.WorkingMemory, component, reason string) {
	if mem == nil {
		return
	}
	if mem.DegradedReasons == nil {
		mem.DegradedReasons = map[string]string{}
	}
	mem.DegradedReasons[component] = reason
}
