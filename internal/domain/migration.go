package domain

import (
	"strconv"
	"strings"
)

var legacyDegradedComponents = []string{
	"pick",
	"eval",
	"critic",
	"refine",
	"probe_ask",
	"probe_eval",
	"reflection",
	"rag",
}

// MigrateLegacyState moves old Notes-based runtime protocols into typed fields.
// Call this after loading a Session from JSON snapshots or state_json.
func (s *Session) MigrateLegacyState() {
	if s == nil || s.WorkingMemory == nil {
		return
	}
	s.WorkingMemory.MigrateLegacyState()
}

// MigrateLegacyState moves old Notes-based runtime protocols into typed fields.
// It is idempotent and never overwrites non-zero typed state.
func (m *WorkingMemory) MigrateLegacyState() {
	if m == nil || len(m.Notes) == 0 {
		return
	}

	if m.ReflectTopic == "" {
		m.ReflectTopic = strings.TrimSpace(m.Notes["reflect_topic"])
	}
	delete(m.Notes, "reflect_topic")

	if m.ScoredRounds == 0 {
		m.ScoredRounds = legacyNoteInt(m.Notes, "scored_rounds")
	}
	delete(m.Notes, "scored_rounds")

	if m.DegradedRounds == 0 {
		m.DegradedRounds = legacyNoteInt(m.Notes, "degraded_rounds")
	}
	delete(m.Notes, "degraded_rounds")

	for _, component := range legacyDegradedComponents {
		m.migrateLegacyDegradedReason(component)
	}
}

func (m *WorkingMemory) migrateLegacyDegradedReason(component string) {
	reasonKey := component + "_degraded_reason"
	flagKey := component + "_degraded"
	reason := strings.TrimSpace(m.Notes[reasonKey])
	flag := strings.TrimSpace(m.Notes[flagKey])

	if reason != "" || flag != "" {
		if m.DegradedReasons == nil {
			m.DegradedReasons = map[string]string{}
		}
		if m.DegradedReasons[component] == "" {
			if reason == "" {
				reason = "legacy degraded marker"
			}
			m.DegradedReasons[component] = reason
		}
	}
	delete(m.Notes, reasonKey)
	delete(m.Notes, flagKey)
}

func legacyNoteInt(notes map[string]string, key string) int {
	n, err := strconv.Atoi(strings.TrimSpace(notes[key]))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
