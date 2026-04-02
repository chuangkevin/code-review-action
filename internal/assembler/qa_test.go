package assembler

import (
	"testing"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

func TestValidateResult_Valid(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "warning", Body: "test"},
		},
		Summary: "Looks good.",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if len(validated.InlineComments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(validated.InlineComments))
	}
}

func TestValidateResult_RemovesInvalidFile(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "warning", Body: "valid"},
			{File: "nonexistent.go", Line: 5, Severity: "critical", Body: "invalid"},
		},
		Summary: "test",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if len(validated.InlineComments) != 1 {
		t.Errorf("expected 1 comment after filtering, got %d", len(validated.InlineComments))
	}
}

func TestValidateResult_EmptySummaryFallback(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role:    "security",
		Summary: "",
	}
	diffFiles := map[string]bool{}

	validated := ValidateResult(result, diffFiles)
	if validated.Summary == "" {
		t.Error("expected fallback summary")
	}
}

func TestValidateResult_FixesSeverity(t *testing.T) {
	result := &reviewer.ReviewResult{
		Role: "backend",
		InlineComments: []reviewer.InlineComment{
			{File: "main.go", Line: 10, Severity: "high", Body: "bad severity"},
		},
		Summary: "ok",
	}
	diffFiles := map[string]bool{"main.go": true}

	validated := ValidateResult(result, diffFiles)
	if validated.InlineComments[0].Severity != "warning" {
		t.Errorf("expected severity fallback to 'warning', got %q", validated.InlineComments[0].Severity)
	}
}
