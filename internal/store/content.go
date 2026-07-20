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
	artifactDigest := digest(content)
	if err := putArtifact(ctx, s.db, artifactDigest, mediaType, content, s.now().UTC().UnixMicro()); err != nil {
		return "", err
	}
	return artifactDigest, nil
}

func (s *Store) Artifact(ctx context.Context, artifactDigest string) (mediaType string, content []byte, err error) {
	return loadArtifact(ctx, s.db, artifactDigest)
}

func loadArtifact(ctx context.Context, query rowQuerier, artifactDigest string) (mediaType string, content []byte, err error) {
	err = query.QueryRowContext(ctx, "SELECT media_type, content FROM artifacts WHERE digest = ?", artifactDigest).
		Scan(&mediaType, &content)
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

type contentWriter interface {
	rowQuerier
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func putArtifact(
	ctx context.Context,
	writer contentWriter,
	artifactDigest, mediaType string,
	content []byte,
	now int64,
) error {
	if protocol.RawDigest(content) != artifactDigest {
		return fmt.Errorf("artifact digest mismatch for %s", artifactDigest)
	}
	if err := protocol.ValidateArtifactContent(mediaType, content); err != nil {
		return err
	}
	content = append([]byte{}, content...)
	if _, err := writer.ExecContext(ctx, `
		INSERT INTO artifacts (digest, media_type, content, size, created_at_us)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(digest) DO NOTHING`,
		artifactDigest, mediaType, content, len(content), now,
	); err != nil {
		return fmt.Errorf("put artifact %s: %w", artifactDigest, err)
	}
	storedType, stored, err := loadArtifact(ctx, writer, artifactDigest)
	if err != nil {
		return err
	}
	if storedType != mediaType || !bytes.Equal(stored, content) {
		return fmt.Errorf("artifact conflict for %s", artifactDigest)
	}
	return nil
}
