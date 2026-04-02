package assembler

import (
	"strings"
	"testing"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

func TestAssemble_Dedup(t *testing.T) {
	results := []*reviewer.ReviewResult{
		{
			Role: "security",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "critical", Body: "SQL injection"},
			},
			Summary: "Found SQL injection.",
		},
		{
			Role: "backend",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "suggestion", Body: "Use parameterized query"},
			},
			Summary: "Consider parameterized queries.",
		},
	}

	output := Assemble(results)

	if len(output.InlineComments) != 1 {
		t.Fatalf("expected 1 merged comment, got %d", len(output.InlineComments))
	}
	if output.InlineComments[0].Severity != "critical" {
		t.Errorf("severity = %q, want critical", output.InlineComments[0].Severity)
	}
	if !strings.Contains(output.InlineComments[0].Body, "Shield") {
		t.Error("merged body should contain Shield")
	}
	if !strings.Contains(output.InlineComments[0].Body, "Rex") {
		t.Error("merged body should contain Rex")
	}
}

func TestAssemble_NoDedup(t *testing.T) {
	results := []*reviewer.ReviewResult{
		{
			Role: "frontend",
			InlineComments: []reviewer.InlineComment{
				{File: "app.vue", Line: 5, Severity: "warning", Body: "CSS issue"},
			},
			Summary: "CSS needs work.",
		},
		{
			Role: "backend",
			InlineComments: []reviewer.InlineComment{
				{File: "main.go", Line: 10, Severity: "warning", Body: "Error handling"},
			},
			Summary: "Add error handling.",
		},
	}

	output := Assemble(results)
	if len(output.InlineComments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(output.InlineComments))
	}
}

func TestBuildSummaryComment(t *testing.T) {
	output := &AssemblyOutput{
		InlineComments: []MergedComment{
			{Severity: "critical", File: "a.go", Line: 1, Body: "x"},
			{Severity: "warning", File: "b.go", Line: 2, Body: "y"},
			{Severity: "suggestion", File: "c.go", Line: 3, Body: "z"},
		},
		Summaries: map[string]string{
			"backend":      "Looks okay.",
			"security":     "No issues.",
			"architecture": "Clean.",
		},
		FailedRoles: []string{"frontend"},
		Skills:      []string{"business-member-doc"},
	}
	pr := reviewer.PRContext{
		Title:  "Test PR",
		Author: "kevin",
		Branch: "feature/test",
	}

	markdown := BuildSummaryComment(output, pr, 42, 5, 10, 3, "https://gitea.example.com/owner/repo/src/branch/main")
	if !strings.Contains(markdown, "Team Discussion") {
		t.Error("summary should contain Team Discussion header")
	}
	if !strings.Contains(markdown, "frontend") {
		t.Error("summary should mention failed role")
	}
	if !strings.Contains(markdown, "business-member-doc") {
		t.Error("summary should list skills used")
	}
}
