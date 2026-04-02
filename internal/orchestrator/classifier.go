package orchestrator

import (
	"path/filepath"
	"strings"
)

type Classification struct {
	Frontend []string
	Backend  []string
	Shared   []string
}

func (c *Classification) HasFrontend() bool {
	return len(c.Frontend) > 0 || len(c.Shared) > 0
}

func (c *Classification) HasBackend() bool {
	return len(c.Backend) > 0 || len(c.Shared) > 0
}

func (c *Classification) AllFiles() []string {
	seen := make(map[string]bool)
	var all []string
	for _, f := range c.Frontend {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	for _, f := range c.Backend {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	for _, f := range c.Shared {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	return all
}

var frontendExts = map[string]bool{
	".vue": true, ".tsx": true, ".jsx": true,
	".css": true, ".scss": true, ".less": true,
	".html": true, ".svelte": true,
}

var backendExts = map[string]bool{
	".go": true, ".cs": true, ".java": true,
	".py": true, ".rs": true, ".rb": true,
}

var frontendDirs = []string{
	"components", "pages", "views", "layouts", "composables",
	"src/components", "src/pages", "src/views",
}

var backendDirs = []string{
	"server", "api", "services", "internal", "cmd",
	"controllers", "handlers", "middleware", "repository",
}

func ClassifyFiles(files []string) *Classification {
	c := &Classification{}

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))

		if frontendExts[ext] {
			c.Frontend = append(c.Frontend, f)
			continue
		}

		if backendExts[ext] {
			c.Backend = append(c.Backend, f)
			continue
		}

		if ext == ".ts" {
			if isInDirs(f, backendDirs) {
				c.Backend = append(c.Backend, f)
			} else if isInDirs(f, frontendDirs) {
				c.Frontend = append(c.Frontend, f)
			} else {
				c.Shared = append(c.Shared, f)
			}
			continue
		}

		c.Shared = append(c.Shared, f)
	}

	return c
}

func isInDirs(filePath string, dirs []string) bool {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	for _, dir := range dirs {
		if strings.HasPrefix(normalized, dir+"/") || strings.Contains(normalized, "/"+dir+"/") {
			return true
		}
	}
	return false
}
