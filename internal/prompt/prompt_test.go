package prompt

import "testing"

func TestClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "fix: handle nil pointer", "fix: handle nil pointer"},
		{"trims", "  feat: add thing\n\n", "feat: add thing"},
		{
			"think block",
			"<think>let me reason\nabout this</think>\nfix: the bug",
			"fix: the bug",
		},
		{
			"code fence",
			"```\nfeat: add parser\n```",
			"feat: add parser",
		},
		{
			"code fence with lang",
			"```text\nchore: bump deps\n```",
			"chore: bump deps",
		},
		{
			"wrapping double quotes",
			"\"fix: off-by-one error\"",
			"fix: off-by-one error",
		},
		{
			"keeps inner quotes when not fully wrapped",
			`fix: handle "quoted" value`,
			`fix: handle "quoted" value`,
		},
		{
			"multiline body preserved",
			"feat: add X\n\nThis adds X because Y.",
			"feat: add X\n\nThis adds X because Y.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Clean(c.in); got != c.want {
				t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
