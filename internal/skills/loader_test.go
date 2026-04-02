package skills

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "skills")
}

func TestLoadSkillIndex(t *testing.T) {
	index, err := LoadSkillIndex(testdataPath())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(index) < 2 {
		t.Fatalf("expected at least 2 skills, got %d", len(index))
	}

	found := false
	for _, s := range index {
		if s.Name == "test-skill-doc" {
			found = true
			if s.Description == "" {
				t.Error("description is empty")
			}
		}
	}
	if !found {
		t.Error("test-skill-doc not found in index")
	}
}

func TestLoadSkillContent(t *testing.T) {
	content, err := LoadSkillContent(testdataPath(), "test-skill-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
}

func TestLoadSkillContent_MultiFile(t *testing.T) {
	content, err := LoadSkillContent(testdataPath(), "multi-skill-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
	if !strings.Contains(content, "Sub Topic") {
		t.Error("content should include sub-topic")
	}
}
