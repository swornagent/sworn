package baton

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mappedContent materialises the exact bytes expected at a mapping's
// destination. Vendor and Diff share this function so they cannot disagree
// about which source types are transformed or copied verbatim.
func mappedContent(sourceDir string, m FileMapping) ([]byte, error) {
	if m.Source == "baton/rules.md" {
		var buf bytes.Buffer
		for _, ruleSrc := range RuleSources() {
			content, err := os.ReadFile(filepath.Join(sourceDir, ruleSrc))
			if err != nil {
				return nil, fmt.Errorf("baton: cannot read rule %s: %w", ruleSrc, err)
			}
			transformed, err := Transform(string(content))
			if err != nil {
				return nil, fmt.Errorf("baton: transform rule %s: %w", ruleSrc, err)
			}
			buf.WriteString(strings.TrimSpace(transformed))
			buf.WriteString("\n\n")
		}
		return []byte(strings.TrimRight(buf.String(), "\n") + "\n"), nil
	}

	content, err := os.ReadFile(filepath.Join(sourceDir, m.Source))
	if err != nil {
		return nil, fmt.Errorf("baton: cannot read %s: %w", m.Source, err)
	}

	if isSchemaSource(m.Source) {
		// Schemas are normative wire contracts. Preserve their bytes exactly;
		// command-reference transformations and newline normalisation are only
		// valid for prose.
		return content, nil
	}

	transformed, err := Transform(string(content))
	if err != nil {
		return nil, fmt.Errorf("baton: transform %s: %w", m.Source, err)
	}
	return []byte(strings.TrimRight(transformed, "\n") + "\n"), nil
}

func isSchemaSource(source string) bool {
	return strings.HasPrefix(filepath.ToSlash(source), "schemas/")
}
