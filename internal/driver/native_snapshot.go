package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
)

// NativeCLISnapshotEventVersion names the canonical body of one run's native
// CLI snapshot fact, journaled once per opted-in adapter before the run's
// first dispatch.
const NativeCLISnapshotEventVersion = "sworn.native-cli-snapshot/v1"

const (
	// nativeCLISnapshotStore is the content-addressed store beneath the
	// machine/user artefact home. A snapshot is durable data a running run
	// depends on, so it never lives under the temp root.
	nativeCLISnapshotStore = "native-cli-snapshots"
	// maxNativeCLISnapshotBytes bounds one copied CLI executable.
	maxNativeCLISnapshotBytes = 1 << 30
	// maxNativeCLISnapshotFactBytes bounds one decoded snapshot fact.
	maxNativeCLISnapshotFactBytes = 16_384
)

// NativeCLISnapshot binds one opted-in native adapter to the exact CLI bytes
// every dispatch in one run executes. SourcePath is the configured host path;
// SnapshotPath is the read-only copy, named by its own digest. A preview for
// the readiness commands has no RunID and names the resolved host file
// itself; it is never recorded and never used by a run.
type NativeCLISnapshot struct {
	SchemaVersion string        `json:"schema_version"`
	RunID         string        `json:"run_id"`
	Adapter       string        `json:"adapter"`
	Family        ProfileFamily `json:"family"`
	SourcePath    string        `json:"source_path"`
	SnapshotPath  string        `json:"snapshot_path"`
	Digest        string        `json:"digest"`
	CLIVersion    string        `json:"cli_version"`
	VersionOutput string        `json:"version_output"`
}

// NativeCLIVersion reads the version a native CLI reports in its --version
// output, without the trailing newline. It is the one parser of the
// per-family format validateNativeConfig checks.
func NativeCLIVersion(family ProfileFamily, output string) (string, bool) {
	var value string
	var ok bool
	switch family {
	case ProfileCodex:
		value, ok = strings.CutPrefix(output, "codex-cli ")
	case ProfileClaude:
		value, ok = strings.CutSuffix(output, " (Claude Code)")
	}
	// Cut returns the input unchanged when it does not match, which would hand
	// a caller a plausible-looking version that was never reported.
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

func EncodeNativeCLISnapshot(snapshot NativeCLISnapshot) ([]byte, error) {
	if err := validateNativeCLISnapshotFact(snapshot); err != nil {
		return nil, err
	}
	return canonicalJSON(snapshot)
}

// DecodeNativeCLISnapshot admits only the exact canonical bytes
// EncodeNativeCLISnapshot produces.
func DecodeNativeCLISnapshot(body []byte) (NativeCLISnapshot, error) {
	var snapshot NativeCLISnapshot
	if _, err := decodeTyped(
		body,
		maxNativeCLISnapshotFactBytes,
		[]string{
			"schema_version", "run_id", "adapter", "family", "source_path",
			"snapshot_path", "digest", "cli_version", "version_output",
		},
		nil,
		&snapshot,
	); err != nil {
		return NativeCLISnapshot{},
			failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid")
	}
	canonical, err := EncodeNativeCLISnapshot(snapshot)
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	if !bytes.Equal(canonical, body) {
		return NativeCLISnapshot{},
			failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid")
	}
	return snapshot, nil
}

func validateNativeCLISnapshotFact(snapshot NativeCLISnapshot) error {
	invalid := failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid")
	if snapshot.SchemaVersion != NativeCLISnapshotEventVersion ||
		!providerKeyPattern.MatchString(snapshot.RunID) ||
		!providerKeyPattern.MatchString(snapshot.Adapter) ||
		(snapshot.Family != ProfileCodex && snapshot.Family != ProfileClaude) ||
		!cleanAbsolutePath(snapshot.SourcePath) ||
		!cleanAbsolutePath(snapshot.SnapshotPath) ||
		!digestPattern.MatchString(snapshot.Digest) ||
		filepath.Base(snapshot.SnapshotPath) !=
			strings.TrimPrefix(snapshot.Digest, "sha256:") ||
		!versionPattern.MatchString(snapshot.CLIVersion) ||
		len(snapshot.VersionOutput) > 256 {
		return invalid
	}
	version, reported := NativeCLIVersion(snapshot.Family, snapshot.VersionOutput)
	if !reported || version != snapshot.CLIVersion {
		return invalid
	}
	return nil
}

func cleanAbsolutePath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}

// VerifyNativeCLISnapshot proves a recorded snapshot still holds exactly the
// bytes its fact names. A missing or changed snapshot is refused, never taken
// again: the run's CLI identity was fixed when the run started.
func VerifyNativeCLISnapshot(snapshot NativeCLISnapshot) error {
	if err := validateNativeCLISnapshotFact(snapshot); err != nil {
		return err
	}
	if _, err := os.Lstat(snapshot.SnapshotPath); errors.Is(err, fs.ErrNotExist) {
		return failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_missing")
	}
	file, err := openPinnedExecutable(ExecutableIdentity{
		Path: snapshot.SnapshotPath, Digest: snapshot.Digest,
	})
	if err != nil {
		return failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_changed")
	}
	_ = file.Close()
	return nil
}

// Profiles returns every configured profile key in canonical order.
func (loaded LoadedDriverConfig) Profiles() []string {
	keys := make([]string, len(loaded.config.Profiles))
	for index := range loaded.config.Profiles {
		keys[index] = loaded.config.Profiles[index].Key
	}
	return keys
}

// NativeCLISnapshotAdapters returns, sorted, the keys of the opted-in native
// adapters the named profiles use. An unknown profile contributes nothing
// here; building the registry refuses it.
func (loaded LoadedDriverConfig) NativeCLISnapshotAdapters(profiles []string) []string {
	used := make(map[string]struct{})
	for _, profile := range loaded.config.Profiles {
		if slices.Contains(profiles, profile.Key) {
			used[profile.Adapter] = struct{}{}
		}
	}
	var keys []string
	for _, raw := range loaded.config.Adapters {
		if raw.Native == nil ||
			raw.Native.CLIResolution != NativeCLIResolutionRunSnapshot {
			continue
		}
		if _, ok := used[raw.Native.Key]; ok {
			keys = append(keys, raw.Native.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// SnapshotNativeCLI resolves one opted-in adapter's host CLI for runID: it
// follows the configured path to the file it names now, copies those bytes
// into the read-only content-addressed store, and captures the copy's
// --version output exactly as dispatch admission runs it. The caller records
// the returned fact before the run's first dispatch and never calls this
// again for the same run and adapter.
func (loaded LoadedDriverConfig) SnapshotNativeCLI(
	ctx context.Context,
	adapter string,
	runID string,
) (NativeCLISnapshot, error) {
	declared, err := loaded.nativeSnapshotDeclaration(adapter)
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	source, err := resolveNativeCLISource(declared.CLI.Path)
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	root, err := nativeCLISnapshotRoot()
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	snapshotPath, digest, err := storeNativeCLISnapshot(root, source)
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	snapshot, err := captureNativeCLISnapshot(
		ctx, declared, runID, snapshotPath, digest,
	)
	if err != nil {
		return NativeCLISnapshot{}, err
	}
	if err := validateNativeCLISnapshotFact(snapshot); err != nil {
		return NativeCLISnapshot{}, err
	}
	return snapshot, nil
}

// PreviewNativeCLISnapshots resolves, without copying, the host CLI of every
// opted-in native adapter the named profiles use, the same way a run started
// now would snapshot it. The readiness commands (inspect, doctor, certify and
// probe) bind these previews so they check and report the CLI a run would
// take.
func (loaded LoadedDriverConfig) PreviewNativeCLISnapshots(
	ctx context.Context,
	profiles []string,
) (map[string]NativeCLISnapshot, error) {
	adapters := loaded.NativeCLISnapshotAdapters(profiles)
	if len(adapters) == 0 {
		return nil, nil
	}
	previews := make(map[string]NativeCLISnapshot, len(adapters))
	for _, adapter := range adapters {
		declared, err := loaded.nativeSnapshotDeclaration(adapter)
		if err != nil {
			return nil, err
		}
		source, err := resolveNativeCLISource(declared.CLI.Path)
		if err != nil {
			return nil, err
		}
		digest, err := executableDigest(source)
		if err != nil {
			return nil, failWithDetail(
				"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "source_unavailable",
			)
		}
		preview, err := captureNativeCLISnapshot(
			ctx, declared, "", source, digest,
		)
		if err != nil {
			return nil, err
		}
		previews[adapter] = preview
	}
	return previews, nil
}

func (loaded LoadedDriverConfig) nativeSnapshotDeclaration(
	adapter string,
) (NativeAdapterConfig, error) {
	for _, raw := range loaded.config.Adapters {
		if raw.Native == nil || raw.Native.Key != adapter ||
			raw.Native.CLIResolution != NativeCLIResolutionRunSnapshot {
			continue
		}
		declared := cloneNativeAdapterConfig(*raw.Native)
		if err := admitNativeConfig(declared); err != nil {
			return NativeAdapterConfig{}, err
		}
		return declared, nil
	}
	return NativeAdapterConfig{},
		failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "adapter_unknown")
}

// bindNativeCLISnapshot admits an opted-in declaration in its own right, then
// binds it to its snapshot. The result is an ordinary pinned configuration
// naming the snapshot's exact bytes, so every later check and launch
// (openNativeClosure's per-dispatch byte check and ExecutedDigest included)
// runs unchanged against the snapshot. An adapter with no snapshot is
// refused: nothing falls back to the host binary.
func bindNativeCLISnapshot(
	declared NativeAdapterConfig,
	snapshots map[string]NativeCLISnapshot,
) (NativeAdapterConfig, error) {
	if err := admitNativeConfig(declared); err != nil {
		return NativeAdapterConfig{}, err
	}
	snapshot, bound := snapshots[declared.Key]
	if !bound {
		return NativeAdapterConfig{},
			failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_unbound")
	}
	if snapshot.Adapter != declared.Key || snapshot.Family != declared.Family ||
		snapshot.SourcePath != declared.CLI.Path {
		return NativeAdapterConfig{},
			failWithDetail("NATIVE_CLI_SNAPSHOT_INVALID", "binding_mismatch")
	}
	declared.CLI = ExecutableIdentity{
		Path: snapshot.SnapshotPath, Digest: snapshot.Digest,
	}
	declared.CLIVersion = snapshot.CLIVersion
	declared.VersionOutput = snapshot.VersionOutput
	declared.CLIResolution = ""
	return declared, nil
}

// resolveNativeCLISource follows the configured host path to the file it
// names now. A launcher symlink an updater repoints is the expected case.
func resolveNativeCLISource(configured string) (string, error) {
	source, err := filepath.EvalSymlinks(configured)
	if err != nil || !cleanAbsolutePath(source) {
		return "", failWithDetail(
			"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "source_unavailable",
		)
	}
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", failWithDetail(
			"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "source_unavailable",
		)
	}
	return source, nil
}

func nativeCLISnapshotRoot() (string, error) {
	paths, err := gitx.LoadHostPaths()
	if err != nil {
		return "", failWithDetail(
			"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "store_unavailable",
		)
	}
	return filepath.Join(paths.ArtefactHome, nativeCLISnapshotStore), nil
}

// storeNativeCLISnapshot copies source into root under the hex of its own
// sha256, read-only. The digest is of the bytes written, so a host file
// replaced mid-copy still yields a self-consistent snapshot. An existing
// entry must already hold exactly those bytes; a corrupted one is refused
// rather than silently replaced.
func storeNativeCLISnapshot(root, source string) (string, string, error) {
	unavailable := func(detail string) (string, string, error) {
		return "", "", failWithDetail("NATIVE_CLI_SNAPSHOT_UNAVAILABLE", detail)
	}
	input, err := os.Open(source)
	if err != nil {
		return unavailable("source_unavailable")
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return unavailable("source_unavailable")
	}
	if info.Size() > maxNativeCLISnapshotBytes {
		return unavailable("source_too_large")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return unavailable("store_unavailable")
	}
	if rootInfo, err := os.Lstat(root); err != nil || !rootInfo.IsDir() {
		return unavailable("store_unavailable")
	}
	partial, err := os.CreateTemp(root, ".partial-")
	if err != nil {
		return unavailable("store_unavailable")
	}
	partialPath := partial.Name()
	defer os.Remove(partialPath)
	hash := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(partial, hash),
		io.LimitReader(input, maxNativeCLISnapshotBytes+1),
	)
	if copyErr == nil {
		copyErr = partial.Chmod(0o555)
	}
	if copyErr == nil {
		copyErr = partial.Sync()
	}
	if closeErr := partial.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return unavailable("store_unavailable")
	}
	if written > maxNativeCLISnapshotBytes {
		return unavailable("source_too_large")
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	target := filepath.Join(root, strings.TrimPrefix(digest, "sha256:"))
	if _, err := os.Lstat(target); err == nil {
		existing, openErr := openPinnedExecutable(ExecutableIdentity{
			Path: target, Digest: digest,
		})
		if openErr != nil {
			return "", "", failWithDetail(
				"NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_changed",
			)
		}
		_ = existing.Close()
		return target, digest, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return unavailable("store_unavailable")
	}
	if err := os.Rename(partialPath, target); err != nil {
		return unavailable("store_unavailable")
	}
	if directory, err := os.Open(root); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return target, digest, nil
}

// captureNativeCLISnapshot runs --version on the pinned candidate exactly as
// doctor and dispatch admission do, and derives the version from what it
// reports.
func captureNativeCLISnapshot(
	ctx context.Context,
	declared NativeAdapterConfig,
	runID string,
	executable string,
	digest string,
) (NativeCLISnapshot, error) {
	pinned := declared
	pinned.CLIResolution = ""
	pinned.CLI = ExecutableIdentity{Path: executable, Digest: digest}
	versionCtx, cancel := context.WithTimeout(ctx, nativeAdmissionProbeBound)
	defer cancel()
	body, err := nativeVersion(versionCtx, pinned)
	defer clearBytes(body)
	if err != nil {
		return NativeCLISnapshot{}, failWithDetail(
			"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "version_unavailable",
		)
	}
	output, terminated := strings.CutSuffix(string(body), "\n")
	version, reported := NativeCLIVersion(declared.Family, output)
	pinned.CLIVersion, pinned.VersionOutput = version, output
	if !terminated || !reported || validateNativeConfig(pinned) != nil {
		return NativeCLISnapshot{}, failWithDetail(
			"NATIVE_CLI_SNAPSHOT_UNAVAILABLE", "version_output",
		)
	}
	return NativeCLISnapshot{
		SchemaVersion: NativeCLISnapshotEventVersion,
		RunID:         runID,
		Adapter:       declared.Key,
		Family:        declared.Family,
		SourcePath:    declared.CLI.Path,
		SnapshotPath:  executable,
		Digest:        digest,
		CLIVersion:    version,
		VersionOutput: output,
	}, nil
}
