package agentkit

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Permission string

const (
	PermissionReadOnly     Permission = "read_only"
	PermissionWriteSession Permission = "write_session"
	PermissionWriteReport  Permission = "write_report"
	PermissionExternalTool Permission = "external_tool"
)

var (
	ErrInvalidSpec = errors.New("agentkit: invalid spec")
	ErrDuplicate   = errors.New("agentkit: duplicate registration")
)

type SkillSpec struct {
	Name          string
	Version       string
	Description   string
	InputSummary  string
	OutputSummary string
	Permission    Permission
	Timeout       time.Duration
}

type SkillRegistry struct {
	byName map[string]SkillSpec
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{byName: map[string]SkillSpec{}}
}

func (r *SkillRegistry) Register(spec SkillSpec) error {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return fmt.Errorf("%w: empty skill name", ErrInvalidSpec)
	}
	spec.Name = name
	if spec.Permission == "" {
		spec.Permission = PermissionReadOnly
	}
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("%w: skill %s", ErrDuplicate, name)
	}
	r.byName[name] = spec
	return nil
}

func (r *SkillRegistry) Get(name string) (SkillSpec, bool) {
	spec, ok := r.byName[strings.TrimSpace(name)]
	return spec, ok
}

func (r *SkillRegistry) List() []SkillSpec {
	out := make([]SkillSpec, 0, len(r.byName))
	for _, spec := range r.byName {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
