package reviewer

import (
	"strings"
	"testing"
)

func TestSplitIntoBatches_SmallDiff(t *testing.T) {
	diff := "diff --git a/file.go b/file.go\n+new line"
	batches := SplitIntoBatches(diff, 100000)
	if len(batches) != 1 {
		t.Errorf("batches = %d, want 1", len(batches))
	}
}

func TestSplitIntoBatches_LargeDiff(t *testing.T) {
	var parts []string
	for i := 0; i < 25; i++ {
		parts = append(parts, "diff --git a/file"+string(rune('a'+i))+".go b/file"+string(rune('a'+i))+".go\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,3 @@\n+new line")
	}
	diff := strings.Join(parts, "\n")

	batches := SplitIntoBatches(diff, 100)
	if len(batches) < 2 {
		t.Errorf("expected multiple batches for 25 files, got %d", len(batches))
	}
	for i, b := range batches {
		count := strings.Count(b, "diff --git")
		if count > 10 {
			t.Errorf("batch %d has %d files, max is 10", i, count)
		}
	}
}

func TestParseDiffFiles(t *testing.T) {
	diff := "diff --git a/internal/config.go b/internal/config.go\n--- a/internal/config.go\n+++ b/internal/config.go\n@@ -1,3 +1,5 @@\n+new line\ndiff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n+another line"

	files := ParseDiffFiles(diff)
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0] != "internal/config.go" {
		t.Errorf("file[0] = %q", files[0])
	}
	if files[1] != "main.go" {
		t.Errorf("file[1] = %q", files[1])
	}
}
