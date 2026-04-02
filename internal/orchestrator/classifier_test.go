package orchestrator

import (
	"testing"
)

func TestClassifyFiles(t *testing.T) {
	files := []string{
		"src/components/Button.vue",
		"src/pages/Home.tsx",
		"internal/service/user.go",
		"api/handler.cs",
		"styles/main.css",
		"README.md",
	}

	result := ClassifyFiles(files)

	if len(result.Frontend) != 3 {
		t.Errorf("frontend = %d, want 3: %v", len(result.Frontend), result.Frontend)
	}
	if len(result.Backend) != 2 {
		t.Errorf("backend = %d, want 2: %v", len(result.Backend), result.Backend)
	}
	if len(result.Shared) != 1 {
		t.Errorf("shared = %d, want 1: %v", len(result.Shared), result.Shared)
	}
}

func TestClassifyFiles_AllFrontend(t *testing.T) {
	files := []string{"app.vue", "style.css"}
	result := ClassifyFiles(files)
	if !result.HasFrontend() {
		t.Error("should have frontend")
	}
	if result.HasBackend() {
		t.Error("should not have backend")
	}
}

func TestClassifyFiles_TSInServerDir(t *testing.T) {
	files := []string{"server/api/routes.ts", "src/components/App.tsx"}
	result := ClassifyFiles(files)
	if len(result.Backend) != 1 || result.Backend[0] != "server/api/routes.ts" {
		t.Errorf("server .ts should be backend: %v", result.Backend)
	}
	if len(result.Frontend) != 1 {
		t.Errorf("component .tsx should be frontend: %v", result.Frontend)
	}
}
