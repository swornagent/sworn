package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/protocol"
)

func (s *Store) Record(ctx context.Context, recordDigest string) (kind string, canonicalJSON []byte, err error) {
	return loadRecord(ctx, s.db, recordDigest)
}

func loadRecord(ctx context.Context, query rowQuerier, recordDigest string) (kind string, canonicalJSON []byte, err error) {
	err = query.QueryRowContext(ctx,
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
