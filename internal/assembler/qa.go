package assembler

import (
	"fmt"

	"github.com/kevinyoung1399/code-review-action/internal/reviewer"
)

var validSeverities = map[string]bool{
	"critical":   true,
	"warning":    true,
	"suggestion": true,
}

func ValidateResult(result *reviewer.ReviewResult, diffFiles map[string]bool) *reviewer.ReviewResult {
	validated := &reviewer.ReviewResult{
		Role:    result.Role,
		Summary: result.Summary,
	}

	if validated.Summary == "" {
		validated.Summary = fmt.Sprintf("%s review 完成，未提供摘要。",
			reviewer.RoleDisplayName(result.Role))
	}

	for _, c := range result.InlineComments {
		if !diffFiles[c.File] {
			continue
		}
		if !validSeverities[c.Severity] {
			c.Severity = "warning"
		}
		if c.Body == "" {
			continue
		}
		if c.Line <= 0 {
			continue
		}
		validated.InlineComments = append(validated.InlineComments, c)
	}

	return validated
}
