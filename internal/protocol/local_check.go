package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/executor"
)

const (
	LocalCheckReceiptSchemaVersion = "sworn-local-check-receipt-v1"
	MaximumLocalCheckReceiptBytes  = 256 << 10
)

type CapturedArtifact struct {
	Ref       string `json:"ref"`
	MediaType string `json:"media_type"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func (artifact CapturedArtifact) Pointer() Artifact {
	return Artifact{Ref: artifact.Ref, MediaType: artifact.MediaType, Digest: artifact.Digest}
}

type LocalCheckReceipt struct {
	SchemaVersion    string           `json:"schema_version"`
	CheckID          string           `json:"check_id"`
	RunID            string           `json:"run_id"`
	Definition       Artifact         `json:"definition"`
	Candidate        CandidatePoint   `json:"candidate"`
	WorkspaceDigest  string           `json:"workspace_digest"`
	Environment      Environment      `json:"environment"`
	WorkspaceAccess  string           `json:"workspace_access"`
	WorkingDirectory string           `json:"working_directory"`
	Argv             []string         `json:"argv"`
	TimeoutSeconds   int64            `json:"timeout_seconds"`
	Network          string           `json:"network"`
	StartedAt        string           `json:"started_at"`
	CompletedAt      string           `json:"completed_at"`
	ExitCode         int              `json:"exit_code"`
	Cancelled        bool             `json:"cancelled"`
	TimedOut         bool             `json:"timed_out"`
	OutputTruncated  bool             `json:"output_truncated"`
	Outcome          string           `json:"outcome"`
	Stdout           CapturedArtifact `json:"stdout"`
	Stderr           CapturedArtifact `json:"stderr"`
}

func EncodeLocalCheckReceipt(receipt LocalCheckReceipt) (EncodedRecord, error) {
	if err := validateLocalCheckReceipt(receipt); err != nil {
		return EncodedRecord{}, err
	}
	canonical, err := EncodeCanonical(receipt)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("canonicalize local check receipt: %w", err)
	}
	return EncodedRecord{
		Kind:          LocalCheckReceiptSchemaVersion,
		CanonicalJSON: canonical,
		Digest:        CanonicalDigest(canonical),
	}, nil
}

func ParseLocalCheckReceipt(contents []byte) (LocalCheckReceipt, error) {
	if len(contents) > MaximumLocalCheckReceiptBytes {
		return LocalCheckReceipt{}, errors.New("local check receipt exceeds byte ceiling")
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return LocalCheckReceipt{}, err
	}
	if !bytes.Equal(contents, canonical) {
		return LocalCheckReceipt{}, errors.New("local check receipt is not canonical JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt LocalCheckReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return LocalCheckReceipt{}, fmt.Errorf("decode local check receipt: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return LocalCheckReceipt{}, errors.New("local check receipt has trailing input")
	}
	if err := validateLocalCheckReceipt(receipt); err != nil {
		return LocalCheckReceipt{}, err
	}
	return receipt, nil
}

func validateLocalCheckReceipt(receipt LocalCheckReceipt) error {
	if receipt.SchemaVersion != LocalCheckReceiptSchemaVersion {
		return fmt.Errorf("unknown local check receipt schema %q", receipt.SchemaVersion)
	}
	if !protocolIDPattern.MatchString(receipt.CheckID) || !protocolIDPattern.MatchString(receipt.RunID) {
		return errors.New("local check receipt requires valid check and run ids")
	}
	if err := validateArtifact(receipt.Definition, "local check definition"); err != nil {
		return err
	}
	if receipt.Definition.MediaType != "application/json" {
		return errors.New("local check definition must be application/json")
	}
	if !nonEmpty(receipt.Candidate.Repository) || !oidPattern.MatchString(receipt.Candidate.Commit) ||
		!oidPattern.MatchString(receipt.Candidate.Tree) {
		return errors.New("local check receipt has an invalid candidate")
	}
	if !digestPattern.MatchString(receipt.WorkspaceDigest) {
		return errors.New("local check receipt has an invalid workspace digest")
	}
	if err := validateEnvironment(receipt.Environment); err != nil || receipt.Environment.Kind != "local" {
		return errors.New("local check receipt requires a concrete local environment")
	}
	if receipt.WorkspaceAccess != "read_only" || receipt.WorkingDirectory != "." || receipt.Network != "none" {
		return errors.New("local check receipt exceeds the initial read-only, root-directory, no-network capability")
	}
	if len(receipt.Argv) == 0 || receipt.TimeoutSeconds <= 0 || receipt.TimeoutSeconds > int64((24*time.Hour)/time.Second) {
		return errors.New("local check receipt requires exact argv")
	}
	if err := executor.ValidateArgv(receipt.Argv); err != nil {
		return fmt.Errorf("local check receipt argv is unsupported: %w", err)
	}
	for _, argument := range receipt.Argv {
		if !nonEmptyArgument(argument) {
			return errors.New("local check receipt contains invalid argv")
		}
	}
	startedAt, err := parseRecordTime(receipt.StartedAt, "local check start")
	if err != nil {
		return err
	}
	completedAt, err := parseRecordTime(receipt.CompletedAt, "local check completion")
	if err != nil || completedAt.Before(startedAt) {
		return errors.New("local check receipt has invalid execution timestamps")
	}
	for name, artifact := range map[string]CapturedArtifact{"stdout": receipt.Stdout, "stderr": receipt.Stderr} {
		if artifact.Size < 0 || artifact.MediaType != "application/octet-stream" {
			return fmt.Errorf("local check %s capture is invalid", name)
		}
		if err := validateArtifact(artifact.Pointer(), "local check "+name); err != nil {
			return err
		}
	}
	controlled := receipt.Cancelled || receipt.TimedOut || receipt.OutputTruncated
	switch receipt.Outcome {
	case "pass":
		if controlled || receipt.ExitCode != 0 {
			return errors.New("passing local check has non-success execution facts")
		}
	case "not_admitted":
		if !controlled && receipt.ExitCode == 0 {
			return errors.New("non-admitted local check has successful execution facts")
		}
	default:
		return fmt.Errorf("invalid local check receipt outcome %q", receipt.Outcome)
	}
	return nil
}

func nonEmptyArgument(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
