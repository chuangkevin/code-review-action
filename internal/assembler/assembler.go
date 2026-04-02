package assembler

import (
	"fmt"
	"strings"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

type MergedComment struct {
	File     string
	Line     int
	Severity string
	Body     string
}

type AssemblyOutput struct {
	InlineComments []MergedComment
	Summaries      map[string]string
	FailedRoles    []string
	Skills         []string
}

var severityRank = map[string]int{
	"critical":   3,
	"warning":    2,
	"suggestion": 1,
}

func Assemble(results []*reviewer.ReviewResult) *AssemblyOutput {
	type key struct {
		File string
		Line int
	}

	merged := make(map[key]*mergeEntry)
	var order []key
	summaries := make(map[string]string)

	for _, r := range results {
		summaries[r.Role] = r.Summary

		for _, c := range r.InlineComments {
			k := key{File: c.File, Line: c.Line}
			if existing, ok := merged[k]; ok {
				existing.addComment(r.Role, c)
			} else {
				entry := &mergeEntry{}
				entry.addComment(r.Role, c)
				merged[k] = entry
				order = append(order, k)
			}
		}
	}

	var comments []MergedComment
	for _, k := range order {
		entry := merged[k]
		comments = append(comments, MergedComment{
			File:     k.File,
			Line:     k.Line,
			Severity: entry.highestSeverity(),
			Body:     entry.buildBody(),
		})
	}

	return &AssemblyOutput{
		InlineComments: comments,
		Summaries:      summaries,
	}
}

type mergeEntry struct {
	parts []struct {
		role    string
		comment reviewer.InlineComment
	}
}

func (m *mergeEntry) addComment(role string, c reviewer.InlineComment) {
	m.parts = append(m.parts, struct {
		role    string
		comment reviewer.InlineComment
	}{role: role, comment: c})
}

func (m *mergeEntry) highestSeverity() string {
	highest := ""
	highestRank := 0
	for _, p := range m.parts {
		rank := severityRank[p.comment.Severity]
		if rank > highestRank {
			highestRank = rank
			highest = p.comment.Severity
		}
	}
	if highest == "" {
		return "warning"
	}
	return highest
}

func (m *mergeEntry) buildBody() string {
	if len(m.parts) == 1 {
		p := m.parts[0]
		return fmt.Sprintf("%s **%s** · %s\n\n%s",
			reviewer.RoleEmoji(p.role),
			reviewer.RoleDisplayName(p.role),
			reviewer.RoleTitle(p.role),
			p.comment.Body)
	}

	var sb strings.Builder
	for i, p := range m.parts {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%s **%s** · %s\n%s",
			reviewer.RoleEmoji(p.role),
			reviewer.RoleDisplayName(p.role),
			reviewer.RoleTitle(p.role),
			p.comment.Body))
	}
	return sb.String()
}

func BuildSummaryComment(output *AssemblyOutput, pr reviewer.PRContext, prNumber, fileCount, additions, deletions int) string {
	var sb strings.Builder

	sb.WriteString("## 🤖 Code Review — Team Discussion\n\n")
	sb.WriteString(fmt.Sprintf("**PR**: #%d %s\n", prNumber, pr.Title))
	sb.WriteString(fmt.Sprintf("**Author**: %s · **Branch**: %s\n", pr.Author, pr.Branch))
	sb.WriteString(fmt.Sprintf("**Files**: %d changed · +%d -%d\n", fileCount, additions, deletions))
	sb.WriteString("\n---\n\n")

	roleOrder := []string{"architecture", "backend", "security", "business", "frontend"}
	for _, role := range roleOrder {
		summary, ok := output.Summaries[role]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("💬 **%s**: %s\n\n", reviewer.RoleDisplayName(role), summary))
	}

	if len(output.FailedRoles) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 以下角色 review 未完成: %s\n\n", strings.Join(output.FailedRoles, ", ")))
	}

	sb.WriteString("---\n\n")

	critical, warning, suggestion := 0, 0, 0
	for _, c := range output.InlineComments {
		switch c.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		case "suggestion":
			suggestion++
		}
	}
	sb.WriteString("| 🔴 Critical | 🟡 Warning | 🔵 Suggestion |\n")
	sb.WriteString("|:-----------:|:----------:|:-------------:|\n")
	sb.WriteString(fmt.Sprintf("| %d | %d | %d |\n", critical, warning, suggestion))

	if len(output.Skills) > 0 {
		sb.WriteString(fmt.Sprintf("\n📚 使用的 Skills: %s\n", strings.Join(output.Skills, ", ")))
	}

	return sb.String()
}
