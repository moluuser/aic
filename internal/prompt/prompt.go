// Package prompt builds the model prompt from git context and cleans the
// model's raw output into a usable commit message.
package prompt

import (
	"fmt"
	"regexp"
	"strings"
)

// Input is the git context used to build the prompt.
type Input struct {
	Branch            string
	StagedFiles       string
	Diff              string
	History           []string
	Language          string
	ExtraInstructions string
	// MaxDiffBytes truncates Diff before it is embedded. 0 disables truncation.
	MaxDiffBytes int
}

// System returns the system / instruction prompt.
func (in Input) System() string {
	lang := in.Language
	if lang == "" {
		lang = "en"
	}

	var b strings.Builder
	b.WriteString(`You are an expert software engineer who writes high-quality git commit messages.
You are given the staged diff, the current branch name, and recent commit messages from this repository.

Write ONE commit message that describes the staged changes.

Rules:
- Match the style, tense, and formatting conventions of the recent commit messages shown. If they use Conventional Commits (feat:, fix:, chore:, ...), follow that. If they use a different style, mirror it.
- The first line is a concise summary, ideally under 72 characters, with no trailing period.
- If the change is non-trivial, add a blank line then a body explaining WHAT changed and WHY, wrapped at ~72 characters. For trivial changes, the summary line alone is fine.
- Describe only what the diff actually changes. Do not invent changes or reference files that are not in the diff.
- Do NOT include markdown code fences, backticks around the whole message, quotes, or any commentary, preamble, or explanation.
- Output ONLY the raw commit message text, nothing else.`)

	fmt.Fprintf(&b, "\n- Write the commit message in this language: %s.", lang)

	if strings.TrimSpace(in.ExtraInstructions) != "" {
		b.WriteString("\n\nProject-specific instructions:\n")
		b.WriteString(strings.TrimSpace(in.ExtraInstructions))
	}
	return b.String()
}

// User returns the user prompt containing the git context.
func (in Input) User() string {
	var b strings.Builder

	if in.Branch != "" {
		fmt.Fprintf(&b, "Current branch: %s\n\n", in.Branch)
	}

	if len(in.History) > 0 {
		b.WriteString("Recent commit messages (newest first), for style reference:\n")
		for _, s := range in.History {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(in.StagedFiles) != "" {
		b.WriteString("Staged files (status\tpath):\n")
		b.WriteString(in.StagedFiles)
		b.WriteString("\n\n")
	}

	diff := in.Diff
	if in.MaxDiffBytes > 0 && len(diff) > in.MaxDiffBytes {
		diff = diff[:in.MaxDiffBytes] + fmt.Sprintf(
			"\n\n... [diff truncated at %d bytes; %d bytes total] ...",
			in.MaxDiffBytes, len(in.Diff))
	}
	b.WriteString("Staged diff:\n")
	b.WriteString(diff)
	b.WriteString("\n\nNow write the commit message.")

	return b.String()
}

var (
	// thinkBlock matches reasoning blocks emitted by models like deepseek-r1.
	thinkBlock = regexp.MustCompile(`(?is)<think>.*?</think>`)
	// fencedBlock matches an opening ``` or ```lang and the closing ```.
	fenceLine = regexp.MustCompile("(?m)^```.*$")
)

// Clean normalises raw model output into a commit message:
//   - strips <think>...</think> reasoning blocks,
//   - removes surrounding markdown code fences,
//   - strips a single layer of wrapping quotes,
//   - trims leading/trailing whitespace.
func Clean(raw string) string {
	s := thinkBlock.ReplaceAllString(raw, "")
	s = fenceLine.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// Remove a single pair of wrapping quotes if the whole message is quoted.
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			inner := strings.TrimSpace(s[1 : len(s)-1])
			if !strings.ContainsAny(inner, "\"'") || s[0] == '\'' {
				s = inner
			}
		}
	}

	return strings.TrimSpace(s)
}
