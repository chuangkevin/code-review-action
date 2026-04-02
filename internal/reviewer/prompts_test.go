package reviewer

import (
	"strings"
	"testing"
)

func TestGetSystemPrompt_AllRoles(t *testing.T) {
	roles := []string{"frontend", "backend", "security", "business", "architecture"}
	for _, role := range roles {
		prompt := GetSystemPrompt(role)
		if prompt == "" {
			t.Errorf("empty prompt for role %q", role)
		}
	}
}

func TestGetSystemPrompt_Frontend_HasPersona(t *testing.T) {
	prompt := GetSystemPrompt("frontend")
	if !strings.Contains(prompt, "Aria") {
		t.Error("frontend prompt should mention Aria")
	}
}

func TestGetSystemPrompt_Unknown(t *testing.T) {
	prompt := GetSystemPrompt("unknown")
	if prompt != "" {
		t.Error("unknown role should return empty prompt")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	prompt := BuildUserPrompt("diff content", []string{"skill content"}, PRContext{
		Title:  "test PR",
		Author: "kevin",
		Branch: "feature/test",
	})
	if !strings.Contains(prompt, "diff content") {
		t.Error("user prompt should contain diff")
	}
	if !strings.Contains(prompt, "skill content") {
		t.Error("user prompt should contain skills")
	}
	if !strings.Contains(prompt, "test PR") {
		t.Error("user prompt should contain PR title")
	}
}

func TestRoleEmoji(t *testing.T) {
	tests := map[string]string{
		"frontend":     "🎨",
		"backend":      "⚙️",
		"security":     "🔒",
		"business":     "💼",
		"architecture": "🏗️",
	}
	for role, expected := range tests {
		if got := RoleEmoji(role); got != expected {
			t.Errorf("RoleEmoji(%q) = %q, want %q", role, got, expected)
		}
	}
}

func TestRoleDisplayName(t *testing.T) {
	if got := RoleDisplayName("frontend"); got != "Aria" {
		t.Errorf("got %q, want Aria", got)
	}
	if got := RoleTitle("frontend"); got != "Senior Frontend Engineer" {
		t.Errorf("got %q, want Senior Frontend Engineer", got)
	}
}
