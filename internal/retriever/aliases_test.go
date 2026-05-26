package retriever

import (
	"reflect"
	"testing"
)

func TestCanonicalizeTags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, nil},
		{"empty_slice", []string{}, nil},
		{"single_alias", []string{"channel"}, []string{"go_concurrency"}},
		{"upper_case", []string{"CHANNEL"}, []string{"go_concurrency"}},
		{"whitespace", []string{"  AOF  "}, []string{"redis_persistence"}},
		{"unknown_passthrough", []string{"kafka"}, []string{"kafka"}},
		{"mixed", []string{"channel", "kafka", "aof"},
			[]string{"go_concurrency", "kafka", "redis_persistence"}},
		{"dedup_after_canon", []string{"channel", "goroutine", "mutex"},
			[]string{"go_concurrency"}},
		{"empty_string_filtered", []string{"", "  ", "channel"},
			[]string{"go_concurrency"}},
		{"preserve_order", []string{"aof", "channel"},
			[]string{"redis_persistence", "go_concurrency"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CanonicalizeTags(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
