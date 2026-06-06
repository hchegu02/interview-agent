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

func workingMemoryWithDegradedReason(mem *domain.WorkingMemory, component, reason string) *domain.WorkingMemory {
	clone := cloneWorkingMemory(mem)
	markDegradedReason(clone, component, reason)
	return clone
}

func cloneWorkingMemory(mem *domain.WorkingMemory) *domain.WorkingMemory {
	if mem == nil {
		return domain.NewWorkingMemory()
	}
	clone := *mem
	clone.ConfirmedSkills = append([]string(nil), mem.ConfirmedSkills...)
	clone.WeakSkills = append([]string(nil), mem.WeakSkills...)
	clone.SuspectedSkills = append([]string(nil), mem.SuspectedSkills...)
	if mem.SkillCoverage != nil {
		clone.SkillCoverage = make(map[string]float64, len(mem.SkillCoverage))
		for skill, coverage := range mem.SkillCoverage {
			clone.SkillCoverage[skill] = coverage
		}
	}
	if mem.Difficulty != nil {
		difficulty := *mem.Difficulty
		clone.Difficulty = &difficulty
	}
	if mem.DegradedReasons != nil {
		clone.DegradedReasons = make(map[string]string, len(mem.DegradedReasons))
		for component, degradedReason := range mem.DegradedReasons {
			clone.DegradedReasons[component] = degradedReason
		}
	}
	if mem.AppliedNodes != nil {
		clone.AppliedNodes = make(map[string]bool, len(mem.AppliedNodes))
		for key, applied := range mem.AppliedNodes {
			clone.AppliedNodes[key] = applied
		}
	}
	if mem.Notes != nil {
		clone.Notes = make(map[string]string, len(mem.Notes))
		for key, value := range mem.Notes {
			clone.Notes[key] = value
		}
	}
	return &clone
}
