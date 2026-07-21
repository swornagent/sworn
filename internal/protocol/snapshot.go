// Package protocol owns the immutable Baton protocol boundary used by Sworn.
package protocol

import (
	"bufio"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

const (
	BatonVersion      = "1.0.0-rc.1"
	BatonSourceCommit = "dd41dcc8c46def2f8b7b86a4f9acd26aeb486667"
	checksumFile      = "snapshot/FILES.sha256"
)

//go:embed snapshot
var embeddedSnapshot embed.FS

// SnapshotSource is the immutable provenance carried with the embedded files.
type SnapshotSource struct {
	Format           string `json:"format"`
	Protocol         string `json:"protocol"`
	ProtocolVersion  string `json:"protocol_version"`
	SourceRepository string `json:"source_repository"`
	SourceCommit     string `json:"source_commit"`
}

// SnapshotFS returns a read-only view rooted at the snapshot directory.
func SnapshotFS() (fs.FS, error) {
	if err := VerifySnapshot(); err != nil {
		return nil, err
	}
	return fs.Sub(embeddedSnapshot, "snapshot")
}

// SnapshotDigest identifies the checksum inventory and therefore every
// admitted snapshot file. VerifySnapshot proves the inventory before use.
func SnapshotDigest() (string, error) {
	contents, err := embeddedSnapshot.ReadFile(checksumFile)
	if err != nil {
		return "", fmt.Errorf("read snapshot checksum inventory: %w", err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), nil
}

// Source returns the provenance record embedded in the snapshot.
func Source() (SnapshotSource, error) {
	contents, err := embeddedSnapshot.ReadFile("snapshot/SOURCE.json")
	if err != nil {
		return SnapshotSource{}, fmt.Errorf("read snapshot source: %w", err)
	}
	var source SnapshotSource
	if err := json.Unmarshal(contents, &source); err != nil {
		return SnapshotSource{}, fmt.Errorf("decode snapshot source: %w", err)
	}
	if source.Format != "sworn-baton-snapshot/v1" ||
		source.Protocol != "Baton" ||
		source.ProtocolVersion != BatonVersion ||
		source.SourceCommit != BatonSourceCommit {
		return SnapshotSource{}, errors.New("snapshot source does not match compiled Baton pin")
	}
	return source, nil
}

// VerifySnapshot checks every embedded file against the admitted inventory and
// rejects unlisted, missing, duplicate, malformed, or changed files.
func VerifySnapshot() error {
	inventory, err := embeddedSnapshot.ReadFile(checksumFile)
	if err != nil {
		return fmt.Errorf("read snapshot checksum inventory: %w", err)
	}

	listed := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(string(inventory)))
	for scanner.Scan() {
		line := scanner.Text()
		digest, name, ok := strings.Cut(line, "  ")
		if !ok || len(digest) != sha256.Size*2 || name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			return fmt.Errorf("malformed snapshot checksum line %q", line)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return fmt.Errorf("malformed snapshot digest for %q: %w", name, err)
		}
		if _, exists := listed[name]; exists {
			return fmt.Errorf("duplicate snapshot inventory path %q", name)
		}
		contents, err := embeddedSnapshot.ReadFile("snapshot/" + name)
		if err != nil {
			return fmt.Errorf("read snapshot file %q: %w", name, err)
		}
		actual := sha256.Sum256(contents)
		if hex.EncodeToString(actual[:]) != digest {
			return fmt.Errorf("snapshot checksum mismatch for %q", name)
		}
		listed[name] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan snapshot checksum inventory: %w", err)
	}

	var embedded []string
	err = fs.WalkDir(embeddedSnapshot, "snapshot", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && path != checksumFile {
			embedded = append(embedded, strings.TrimPrefix(path, "snapshot/"))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk embedded snapshot: %w", err)
	}
	slices.Sort(embedded)
	if len(embedded) != len(listed) {
		return fmt.Errorf("snapshot inventory lists %d files but embeds %d", len(listed), len(embedded))
	}
	for _, name := range embedded {
		if _, ok := listed[name]; !ok {
			return fmt.Errorf("embedded snapshot file %q is not inventoried", name)
		}
	}
	_, err = Source()
	return err
}
