package reviewer

import (
	"strings"
)

const maxFilesPerBatch = 10

func SplitIntoBatches(diff string, maxDiffSize int) []string {
	if len(diff) <= maxDiffSize {
		return []string{diff}
	}

	fileDiffs := splitByFile(diff)
	if len(fileDiffs) <= maxFilesPerBatch {
		return []string{diff}
	}

	var batches []string
	for i := 0; i < len(fileDiffs); i += maxFilesPerBatch {
		end := i + maxFilesPerBatch
		if end > len(fileDiffs) {
			end = len(fileDiffs)
		}
		batch := strings.Join(fileDiffs[i:end], "\n")
		batches = append(batches, batch)
	}
	return batches
}

func ParseDiffFiles(diff string) []string {
	var files []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				path := strings.TrimPrefix(parts[3], "b/")
				files = append(files, path)
			}
		}
	}
	return files
}

func splitByFile(diff string) []string {
	lines := strings.Split(diff, "\n")
	var chunks []string
	var current []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, "\n"))
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}
	return chunks
}
