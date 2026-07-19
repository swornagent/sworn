package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/protocol"
)

func (s *Store) PutRecord(ctx context.Context, kind string, canonicalJSON []byte) (string, error) {
	if s.readOnly {
		return "", errors.New("control store is read-only")
	}
	if strings.TrimSpace(kind) == "" || !json.Valid(canonicalJSON) {
		return "", errors.New("record requires a kind and valid canonical JSON")
	}
	normalized, err := protocol.CanonicalizeJSON(canonicalJSON)
	if err != nil {
		return "", fmt.Errorf("record requires strict I-JSON: %w", err)
	}
	if !bytes.Equal(normalized, canonicalJSON) {
		return "", errors.New("record JSON is not RFC 8785 canonical")
	}
	recordDigest := digest(canonicalJSON)
	now := s.now().UTC().UnixMicro()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO NOTHING`,
		recordDigest, kind, canonicalJSON, len(canonicalJSON), now,
	); err != nil {
		return "", fmt.Errorf("put record %s: %w", recordDigest, err)
	}
	var storedKind string
	var stored []byte
	if err := s.db.QueryRowContext(ctx,
		"SELECT kind, canonical_json FROM records WHERE digest = ?", recordDigest,
	).Scan(&storedKind, &stored); err != nil {
		return "", fmt.Errorf("verify record %s: %w", recordDigest, err)
	}
	if storedKind != kind || !bytes.Equal(stored, canonicalJSON) {
		return "", fmt.Errorf("record digest collision or kind conflict for %s", recordDigest)
	}
	return recordDigest, nil
}

func (s *Store) Record(ctx context.Context, recordDigest string) (kind string, canonicalJSON []byte, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT kind, canonical_json FROM records WHERE digest = ?", recordDigest,
	).Scan(&kind, &canonicalJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("record %s: %w", recordDigest, sql.ErrNoRows)
	}
	if err != nil {
		return "", nil, fmt.Errorf("read record %s: %w", recordDigest, err)
	}
	if digest(canonicalJSON) != recordDigest {
		return "", nil, fmt.Errorf("record %s content digest mismatch", recordDigest)
	}
	return kind, canonicalJSON, nil
}

func (s *Store) PutArtifact(ctx context.Context, mediaType string, content []byte) (string, error) {
	if s.readOnly {
		return "", errors.New("control store is read-only")
	}
	if err := protocol.ValidateArtifactContent(mediaType, content); err != nil {
		return "", err
	}
	// database/sql maps a nil byte slice to SQL NULL. Empty artifact bytes are
	// valid and must remain an empty BLOB under their well-known SHA-256 digest.
	content = append([]byte{}, content...)
	artifactDigest := digest(content)
	now := s.now().UTC().UnixMicro()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts (digest, media_type, content, size, created_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO NOTHING`,
		artifactDigest, mediaType, content, len(content), now,
	); err != nil {
		return "", fmt.Errorf("put artifact %s: %w", artifactDigest, err)
	}
	var storedType string
	var stored []byte
	if err := s.db.QueryRowContext(ctx,
		"SELECT media_type, content FROM artifacts WHERE digest = ?", artifactDigest,
	).Scan(&storedType, &stored); err != nil {
		return "", fmt.Errorf("verify artifact %s: %w", artifactDigest, err)
	}
	if storedType != mediaType || !bytes.Equal(stored, content) {
		return "", fmt.Errorf("artifact digest collision or media-type conflict for %s", artifactDigest)
	}
	return artifactDigest, nil
}

func (s *Store) Artifact(ctx context.Context, artifactDigest string) (mediaType string, content []byte, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT media_type, content FROM artifacts WHERE digest = ?", artifactDigest,
	).Scan(&mediaType, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("artifact %s: %w", artifactDigest, sql.ErrNoRows)
	}
	if err != nil {
		return "", nil, fmt.Errorf("read artifact %s: %w", artifactDigest, err)
	}
	if digest(content) != artifactDigest {
		return "", nil, fmt.Errorf("artifact %s content digest mismatch", artifactDigest)
	}
	return mediaType, content, nil
}
