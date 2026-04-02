package skills

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func CloneSkillsRepo(repoURL, token string) (string, error) {
	dir, err := os.MkdirTemp("", "skills-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	cloneURL := repoURL
	if token != "" {
		cloneURL = strings.Replace(repoURL, "://", "://"+token+"@", 1)
	}

	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w", err)
	}

	return filepath.Join(dir, "skills"), nil
}

func LoadSkillIndex(skillsDir string) ([]SkillEntry, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var index []SkillEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue
		}

		name, desc, err := parseFrontmatter(skillFile)
		if err != nil {
			continue
		}

		index = append(index, SkillEntry{
			Name:        name,
			Description: desc,
		})
	}
	return index, nil
}

func LoadSkillContent(skillsDir, skillName string) (string, error) {
	skillDir := filepath.Join(skillsDir, skillName)
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return "", fmt.Errorf("read skill dir %s: %w", skillName, err)
	}

	var parts []string

	skillFile := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	parts = append(parts, string(content))

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SKILL.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(skillDir, entry.Name()))
		if err != nil {
			continue
		}
		parts = append(parts, string(content))
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

func parseFrontmatter(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	name := ""
	desc := ""

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			continue
		}

		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		}
		if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.Trim(desc, "\"'")
		}
	}

	if name == "" {
		return "", "", fmt.Errorf("no name in frontmatter")
	}
	return name, desc, nil
}
