package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/swornagent/sworn/internal/protocol"
)

// SubmissionRecord reads the immutable canonical record bound to one global
// submission ID.
func (s *Store) SubmissionRecord(ctx context.Context, submissionID string) (string, []byte, error) {
	var digest string
	var kind string
	var canonical []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT identities.digest, records.kind, records.canonical_json
		FROM submission_records AS identities
		JOIN records ON records.digest = identities.digest
		WHERE identities.submission_id = ?`, submissionID,
	).Scan(&digest, &kind, &canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("submission %q: %w", submissionID, sql.ErrNoRows)
	}
	if err != nil {
		return "", nil, fmt.Errorf("read submission %q: %w", submissionID, err)
	}
	if kind != protocol.SubmissionSchemaVersion || protocol.RawDigest(canonical) != digest {
		return "", nil, errors.New("stored submission record binding is invalid")
	}
	return digest, canonical, nil
}

func submissionArtifacts(submission protocol.Submission) ([]protocol.Artifact, error) {
	artifacts := []protocol.Artifact{submission.AuthorityReceipt}
	seen := map[string]string{submission.AuthorityReceipt.Digest: submission.AuthorityReceipt.MediaType}
	for _, check := range submission.Checks {
		if mediaType, exists := seen[check.Receipt.Digest]; exists && mediaType != check.Receipt.MediaType {
			return nil, fmt.Errorf("submission reuses artifact %s with conflicting media types", check.Receipt.Digest)
		} else if !exists {
			seen[check.Receipt.Digest] = check.Receipt.MediaType
			artifacts = append(artifacts, check.Receipt)
		}
	}
	for _, evidence := range submission.Evidence {
		if mediaType, exists := seen[evidence.Artifact.Digest]; exists && mediaType != evidence.Artifact.MediaType {
			return nil, fmt.Errorf("submission reuses artifact %s with conflicting media types", evidence.Artifact.Digest)
		} else if !exists {
			seen[evidence.Artifact.Digest] = evidence.Artifact.MediaType
			artifacts = append(artifacts, evidence.Artifact)
		}
	}
	return artifacts, nil
}

func completeSubmissionArtifacts(
	submission protocol.Submission,
	dependencies []protocol.Artifact,
) ([]protocol.Artifact, error) {
	if len(dependencies) == 0 {
		return nil, errors.New("prepared submission lacks its resolved artifact closure")
	}
	all := make(map[string]protocol.Artifact, len(dependencies))
	for _, pointer := range dependencies {
		if existing, exists := all[pointer.Digest]; exists {
			if existing != pointer {
				return nil, fmt.Errorf("prepared artifact %s has conflicting pointers", pointer.Digest)
			}
			continue
		}
		all[pointer.Digest] = pointer
	}
	direct, err := submissionArtifacts(submission)
	if err != nil {
		return nil, err
	}
	for _, pointer := range direct {
		if dependency, exists := all[pointer.Digest]; !exists || dependency != pointer {
			return nil, fmt.Errorf("prepared submission omits direct artifact %s", pointer.Digest)
		}
	}
	ordered := make([]protocol.Artifact, 0, len(all))
	for _, pointer := range all {
		ordered = append(ordered, pointer)
	}
	slices.SortFunc(ordered, func(left, right protocol.Artifact) int {
		return cmp.Compare(left.Digest, right.Digest)
	})
	return ordered, nil
}

func verifySubmissionArtifact(ctx context.Context, transaction *sql.Tx, pointer protocol.Artifact) ([]byte, error) {
	if pointer.Ref != pointer.Digest {
		return nil, fmt.Errorf("submission artifact %s is not a CAS reference", pointer.Digest)
	}
	var mediaType string
	var contents []byte
	err := transaction.QueryRowContext(ctx,
		"SELECT media_type, content FROM artifacts WHERE digest = ?", pointer.Digest,
	).Scan(&mediaType, &contents)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("submission artifact %s is unavailable", pointer.Digest)
	}
	if err != nil {
		return nil, fmt.Errorf("read submission artifact %s: %w", pointer.Digest, err)
	}
	if mediaType != pointer.MediaType || protocol.RawDigest(contents) != pointer.Digest {
		return nil, fmt.Errorf("submission artifact %s does not match its pointer", pointer.Digest)
	}
	return contents, nil
}
