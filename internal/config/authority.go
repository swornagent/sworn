// Package config composes immutable process configuration into Sworn services.
package config

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	// AuthorityBundleSchemaVersion identifies the local transport envelope which
	// carries exact authority source and detached proof bytes. It is process
	// configuration, not a Baton record schema.
	AuthorityBundleSchemaVersion = "sworn-authority-bundle-v1"

	maximumAuthorityBundleBytes = 108 << 10
)

// AuthoritySource pins one logical source to an authorizer verification key
// and a trusted directory of exact-plan bundles. BundleDirectory must be an
// existing clean absolute directory outside autonomous runner write scope.
// PublicKey is verification material only; private signing keys never enter
// this configuration.
type AuthoritySource struct {
	SourceRef       string
	AuthorizerRef   string
	PublicKey       ed25519.PublicKey
	BundleDirectory string
}

// Authority owns the production policy service and the retained directory
// roots used by its resolver. Call Close when the process composition ends.
type Authority struct {
	service  *policy.Authority
	resolver *authorityBundleResolver
}

// OpenAuthority fixes trust roots and authority bundle directories for one
// process lifetime. Per-operation input can select only an exact configured
// source reference and exact plan digest; it cannot replace this configuration.
func OpenAuthority(
	sources []AuthoritySource,
	ledger policy.ApprovalLedger,
) (*Authority, error) {
	if len(sources) == 0 {
		return nil, errors.New("authority configuration requires at least one source")
	}
	if err := validateAuthorityPlatform(); err != nil {
		return nil, err
	}

	type preparedSource struct {
		ref       string
		directory string
		root      policy.TrustRoot
	}
	prepared := make([]preparedSource, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for index, source := range sources {
		ref := source.SourceRef
		directory := source.BundleDirectory
		if _, exists := seen[ref]; exists {
			return nil, fmt.Errorf("authority source %d duplicates source reference %q", index, ref)
		}
		root, err := policy.NewTrustRoot(ref, source.AuthorizerRef, source.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("authority source %d trust root: %w", index, err)
		}
		if err := validateBundleDirectory(directory); err != nil {
			return nil, fmt.Errorf("authority source %q bundle directory: %w", ref, err)
		}
		seen[ref] = struct{}{}
		prepared = append(prepared, preparedSource{ref: ref, directory: directory, root: root})
	}

	resolver := &authorityBundleResolver{sources: make(map[string]*os.Root, len(prepared))}
	roots := make([]policy.TrustRoot, 0, len(prepared))
	for _, source := range prepared {
		root, err := openBundleRoot(source.directory)
		if err != nil {
			return nil, errors.Join(
				fmt.Errorf("open authority source %q bundle directory: %w", source.ref, err),
				resolver.Close(),
			)
		}
		resolver.sources[source.ref] = root
		roots = append(roots, source.root)
	}
	service, err := policy.NewAuthority(roots, resolver, ledger)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("configure authority service: %w", err), resolver.Close())
	}
	return &Authority{service: service, resolver: resolver}, nil
}

// Service returns the configured policy service while this composition is
// open. A previously retained service also fails closed through its resolver
// after Close.
func (authority *Authority) Service() *policy.Authority {
	if authority == nil || authority.service == nil || authority.resolver == nil || authority.resolver.isClosed() {
		return nil
	}
	return authority.service
}

// Close permanently closes every retained bundle-directory root. It is safe to
// call more than once.
func (authority *Authority) Close() error {
	if authority == nil || authority.resolver == nil {
		return nil
	}
	return authority.resolver.Close()
}

// authorityBundleResolver owns its retained roots. The read lock keeps them
// open through one complete resolution and makes Close a terminal boundary.
type authorityBundleResolver struct {
	mu      sync.RWMutex
	sources map[string]*os.Root
	closed  bool
}

func (resolver *authorityBundleResolver) Resolve(
	ctx context.Context,
	sourceRef string,
	planDigest string,
) ([]byte, []byte, error) {
	if resolver == nil {
		return nil, nil, errors.New("authority bundle resolver is not initialized")
	}
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	if resolver.closed {
		return nil, nil, errors.New("authority bundle resolver is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	root, exists := resolver.sources[sourceRef]
	if !exists || root == nil {
		return nil, nil, fmt.Errorf("authority source %q is not configured", sourceRef)
	}
	filename, err := authorityBundleFilename(planDigest)
	if err != nil {
		return nil, nil, err
	}
	contents, err := readAuthorityBundle(ctx, root, filename)
	if err != nil {
		return nil, nil, fmt.Errorf("read authority bundle for source %q and plan %q: %w", sourceRef, planDigest, err)
	}
	resolvedSource, proof, err := decodeAuthorityBundle(contents)
	if err != nil {
		return nil, nil, fmt.Errorf("decode authority bundle for source %q and plan %q: %w", sourceRef, planDigest, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return resolvedSource, proof, nil
}

func (resolver *authorityBundleResolver) isClosed() bool {
	if resolver == nil {
		return true
	}
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	return resolver.closed
}

func (resolver *authorityBundleResolver) Close() error {
	if resolver == nil {
		return nil
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.closed {
		return nil
	}
	resolver.closed = true
	var closeErr error
	for _, root := range resolver.sources {
		if root != nil {
			closeErr = errors.Join(closeErr, root.Close())
		}
	}
	return closeErr
}

func validateBundleDirectory(directory string) error {
	if directory == "" || strings.IndexByte(directory, 0) >= 0 || !filepath.IsAbs(directory) {
		return errors.New("path must be a non-empty absolute path")
	}
	if filepath.Clean(directory) != directory {
		return errors.New("path must be clean")
	}
	return nil
}

func openBundleRoot(directory string) (*os.Root, error) {
	before, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("inspect directory: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, errors.New("path is not a direct directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	opened, openedErr := root.Stat(".")
	current, currentErr := os.Lstat(directory)
	if openedErr != nil || currentErr != nil || !opened.IsDir() || current.Mode()&os.ModeSymlink != 0 ||
		!current.IsDir() || !os.SameFile(before, opened) || !os.SameFile(opened, current) {
		return nil, errors.Join(errors.New("bundle directory changed while being retained"), openedErr, currentErr, root.Close())
	}
	return root, nil
}

func authorityBundleFilename(planDigest string) (string, error) {
	if !protocol.ValidDigest(planDigest) {
		return "", errors.New("authority bundle requires a canonical SHA-256 plan digest")
	}
	return strings.TrimPrefix(planDigest, "sha256:") + ".json", nil
}

func readAuthorityBundle(
	ctx context.Context,
	root *os.Root,
	filename string,
) (_ []byte, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := root.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if err := validateBundleFileInfo(before); err != nil {
		return nil, err
	}
	file, err := openBundleFile(root, filename)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()

	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := validateBundleFileInfo(opened); err != nil {
		return nil, err
	}
	if !os.SameFile(before, opened) || before.Size() != opened.Size() {
		return nil, errors.New("authority bundle changed while being opened")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumAuthorityBundleBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 || len(contents) > maximumAuthorityBundleBytes {
		return nil, errors.New("authority bundle is empty or exceeds its byte ceiling")
	}
	if opened.Size() != int64(len(contents)) {
		return nil, errors.New("authority bundle changed while being read")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return contents, nil
}

func validateBundleFileInfo(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("authority bundle must be a non-symlink regular file")
	}
	if info.Size() <= 0 || info.Size() > maximumAuthorityBundleBytes {
		return errors.New("authority bundle is empty or exceeds its byte ceiling")
	}
	if err := requireSingleLink(info); err != nil {
		return fmt.Errorf("authority bundle link count: %w", err)
	}
	return nil
}

type encodedAuthorityBundle struct {
	SchemaVersion string `json:"schema_version"`
	Source        string `json:"source"`
	Proof         string `json:"proof"`
}

func decodeAuthorityBundle(contents []byte) ([]byte, []byte, error) {
	if len(contents) == 0 || len(contents) > maximumAuthorityBundleBytes {
		return nil, nil, errors.New("authority bundle is empty or exceeds its byte ceiling")
	}
	canonical, err := protocol.CanonicalizeJSON(contents)
	if err != nil {
		return nil, nil, fmt.Errorf("authority bundle is not strict I-JSON: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &object); err != nil {
		return nil, nil, fmt.Errorf("decode authority bundle object: %w", err)
	}
	required := []string{"schema_version", "source", "proof"}
	allowed := make(map[string]struct{}, len(required))
	for _, name := range required {
		allowed[name] = struct{}{}
		if _, exists := object[name]; !exists {
			return nil, nil, fmt.Errorf("authority bundle is missing field %q", name)
		}
	}
	for name := range object {
		if _, exists := allowed[name]; !exists {
			return nil, nil, fmt.Errorf("authority bundle contains unknown field %q", name)
		}
	}
	var bundle encodedAuthorityBundle
	if err := json.Unmarshal(canonical, &bundle); err != nil {
		return nil, nil, fmt.Errorf("decode authority bundle: %w", err)
	}
	if bundle.SchemaVersion != AuthorityBundleSchemaVersion {
		return nil, nil, fmt.Errorf("unknown authority bundle schema %q", bundle.SchemaVersion)
	}
	source, err := decodeBundleMember("source", bundle.Source, policy.MaximumAuthoritySourceBytes)
	if err != nil {
		return nil, nil, err
	}
	proof, err := decodeBundleMember("proof", bundle.Proof, policy.MaximumAuthorityProofBytes)
	if err != nil {
		return nil, nil, err
	}
	return source, proof, nil
}

func decodeBundleMember(label, encoded string, maximum int) ([]byte, error) {
	if encoded == "" || len(encoded) > base64.RawURLEncoding.EncodedLen(maximum) {
		return nil, fmt.Errorf("authority bundle %s is empty or exceeds its byte ceiling", label)
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, fmt.Errorf("authority bundle %s is not canonical base64url", label)
	}
	if len(decoded) == 0 || len(decoded) > maximum {
		return nil, fmt.Errorf("authority bundle %s is empty or exceeds its byte ceiling", label)
	}
	return decoded, nil
}
