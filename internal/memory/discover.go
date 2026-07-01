package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DiscoveredEntry represents an entry found on disk before embedding.
type DiscoveredEntry struct {
	Path    string
	Harness string
	Title   string
	Content string
}

// DiscoverEntries scans the configured paths and returns all discovered entries.
func DiscoverEntries(cfg *MemoryConfig, cwd string) ([]DiscoveredEntry, error) {
	var all []DiscoveredEntry

	for _, h := range cfg.Harnesses {
		entries, err := discoverHarness(HarnessID(h), cwd)
		if err != nil {
			// If a harness path doesn't exist, we just skip it.
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("discovering harness %s: %w", h, err)
		}
		all = append(all, entries...)
	}

	for _, p := range cfg.ExtraPaths {
		entries, err := discoverCustomPath(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("discovering custom path %s: %w", p, err)
		}
		all = append(all, entries...)
	}

	return all, nil
}

func discoverHarness(h HarnessID, cwd string) ([]DiscoveredEntry, error) {
	path := HarnessMemoryPath(h, cwd)
	if path == "" {
		return nil, nil
	}

	switch h {
	case HarnessClaudeCode:
		return discoverClaudeCode(path)
	case HarnessGeminiCLI, HarnessOpenCode, HarnessCursor, HarnessWindsurf, HarnessCodex:
		return discoverFlatFile(path, string(h))
	default:
		return nil, fmt.Errorf("unsupported harness for discovery: %s", h)
	}
}

var claudeLinkRegex = regexp.MustCompile(`^-\s+\[([^\]]+)\]\(([^)]+)\)`)

func discoverClaudeCode(memoryDir string) ([]DiscoveredEntry, error) {
	indexFile := filepath.Join(memoryDir, "MEMORY.md")
	f, err := os.Open(indexFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []DiscoveredEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := claudeLinkRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			title := matches[1]
			relPath := matches[2]
			absPath := filepath.Join(memoryDir, relPath)

			content, err := os.ReadFile(absPath)
			if err != nil {
				// Skip unreadable linked files
				continue
			}

			entries = append(entries, DiscoveredEntry{
				Path:    absPath,
				Harness: string(HarnessClaudeCode),
				Title:   title,
				Content: string(content),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func discoverFlatFile(path string, harness string) ([]DiscoveredEntry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := string(content)
	parts := strings.Split(text, "\n---\n")

	var entries []DiscoveredEntry
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		title := ""
		lines := strings.SplitN(part, "\n", 2)
		if len(lines) > 0 {
			title = strings.TrimSpace(strings.TrimPrefix(lines[0], "#"))
		}

		entryPath := path
		if len(parts) > 1 {
			entryPath = fmt.Sprintf("%s#%d", path, i)
		}

		entries = append(entries, DiscoveredEntry{
			Path:    entryPath,
			Harness: harness,
			Title:   title,
			Content: part,
		})
	}

	return entries, nil
}

func discoverCustomPath(path string) ([]DiscoveredEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return discoverFlatFile(path, string(HarnessCustom))
	}

	var entries []DiscoveredEntry
	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}

		title := ""
		lines := strings.SplitN(string(content), "\n", 2)
		if len(lines) > 0 {
			title = strings.TrimSpace(strings.TrimPrefix(lines[0], "#"))
		}

		entries = append(entries, DiscoveredEntry{
			Path:    p,
			Harness: string(HarnessCustom),
			Title:   title,
			Content: string(content),
		})
		return nil
	})

	return entries, err
}
