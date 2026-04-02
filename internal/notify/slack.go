package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kevinyoung1399/code-review-action/internal/assembler"
)

func ShouldNotify(strategy string, output *assembler.AssemblyOutput) bool {
	switch strategy {
	case "always":
		return true
	case "off":
		return false
	case "on_issues":
		for _, c := range output.InlineComments {
			if c.Severity == "critical" || c.Severity == "warning" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func SendSlack(webhookURL string, output *assembler.AssemblyOutput, prURL, prTitle, author string) error {
	critical, warning, suggestion := countSeverities(output)
	text := buildSlackMessage(output, prURL, prTitle, author, critical, warning, suggestion)

	payload, _ := json.Marshal(map[string]string{"text": text})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func buildSlackMessage(output *assembler.AssemblyOutput, prURL, prTitle, author string, critical, warning, suggestion int) string {
	var sb strings.Builder

	sb.WriteString("🤖 *Code Review 完成*\n\n")
	sb.WriteString(fmt.Sprintf("*PR*: <%s|%s>\n", prURL, prTitle))
	sb.WriteString(fmt.Sprintf("*Author*: %s\n\n", author))
	sb.WriteString(fmt.Sprintf("🔴 Critical: %d\n", critical))
	sb.WriteString(fmt.Sprintf("🟡 Warning: %d\n", warning))
	sb.WriteString(fmt.Sprintf("🔵 Suggestion: %d\n", suggestion))

	var criticals []assembler.MergedComment
	for _, c := range output.InlineComments {
		if c.Severity == "critical" {
			criticals = append(criticals, c)
		}
	}
	if len(criticals) > 0 {
		sb.WriteString("\n*重點發現:*\n")
		limit := 5
		if len(criticals) < limit {
			limit = len(criticals)
		}
		for _, c := range criticals[:limit] {
			firstLine := strings.SplitN(c.Body, "\n", 2)[0]
			if len(firstLine) > 80 {
				firstLine = firstLine[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("• %s (%s:%d)\n", firstLine, c.File, c.Line))
		}
	}

	sb.WriteString(fmt.Sprintf("\n<%s|查看完整 Review →>", prURL))
	return sb.String()
}

func countSeverities(output *assembler.AssemblyOutput) (critical, warning, suggestion int) {
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
	return
}
