package session

import (
	"testing"

	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
)

func TestEvidence(t *testing.T) {
	steps := []model.Step{
		{Seq: 1, StepType: "env_check"},
		{Seq: 2, StepType: "query", ResultClass: "no_permission"},
		{Seq: 3, StepType: "query", ResultClass: "ok"},
		{Seq: 4, StepType: "note", ResultClass: "ok"},
	}
	if !hasQueryEvidence(steps) {
		t.Fatal("session with an ok query should have evidence")
	}
	if hasQueryEvidence(steps[:2]) {
		t.Fatal("no_permission is not evidence")
	}

	cases := []struct {
		cited []int
		want  bool
	}{
		{nil, false},
		{[]int{1}, false},   // env_check
		{[]int{2}, false},   // no_permission
		{[]int{4}, false},   // a note can't be evidence even with result ok
		{[]int{9}, false},   // not in this session
		{[]int{2, 3}, true}, // one valid step is enough
		{[]int{3}, true},
	}
	for _, c := range cases {
		if got := validEvidenceSteps(c.cited, steps); got != c.want {
			t.Errorf("validEvidenceSteps(%v) = %v, want %v", c.cited, got, c.want)
		}
	}
}

func TestNormalizeRepo(t *testing.T) {
	cases := map[string]string{
		"git@github.com:shop/checkout-web.git":                "shop/checkout-web",
		"https://user:token@github.com/shop/checkout-web.git": "shop/checkout-web",
		"ssh://git@example.com:2222/team/sub/repo.git/":       "team/sub/repo",
		"https://github.com/solo":                             "",
		"not-a-remote":                                        "",
		"":                                                    "",
	}
	for in, want := range cases {
		if got := normalizeRepo(in); got != want {
			t.Errorf("normalizeRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
