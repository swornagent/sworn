package config

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	testAuthoritySourceRef     = "authority:production"
	testAuthorityAuthorizerRef = "identity:production-owner"
	testAuthorityPlanDigest    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type discardAuthorityLedger struct{}

func (*discardAuthorityLedger) PutAuthoritySource(context.Context, policy.PreparedSource) error {
	return nil
}

func (*discardAuthorityLedger) PutAuthorityApproval(context.Context, policy.PreparedApproval) error {
	return nil
}

func TestAuthorityBundleResolverReadsFreshExactBundlesFromRetainedRoot(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "bundles")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	firstSource, firstProof := []byte("source-one\n"), []byte("proof-one\n")
	writeAuthorityBundle(t, directory, testAuthorityPlanDigest, firstSource, firstProof)

	configuration := testAuthorityConfiguration(directory)
	authority, err := OpenAuthority(configuration, &discardAuthorityLedger{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := authority.Close(); err != nil {
			t.Error(err)
		}
	})
	if authority.Service() == nil {
		t.Fatal("open authority did not expose its policy service")
	}
	resolver := authority.resolver

	// Caller-owned startup inputs cannot mutate the retained configuration.
	configuration[0].SourceRef = "authority:replaced"
	configuration[0].AuthorizerRef = "identity:replaced"
	configuration[0].BundleDirectory = filepath.Join(parent, "elsewhere")
	configuration[0].PublicKey[0] ^= 0xff
	assertResolvedBundle(t, resolver, testAuthoritySourceRef, firstSource, firstProof)

	secondSource, secondProof := []byte("source-two\x00"), []byte("proof-two\x00")
	writeAuthorityBundle(t, directory, testAuthorityPlanDigest, secondSource, secondProof)
	assertResolvedBundle(t, resolver, testAuthoritySourceRef, secondSource, secondProof)

	// OpenRoot retains the configured directory identity. Replacing its startup
	// pathname cannot redirect later resolutions to a different directory.
	retainedDirectory := filepath.Join(parent, "retained-bundles")
	if err := os.Rename(directory, retainedDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeAuthorityBundle(t, directory, testAuthorityPlanDigest, []byte("redirected"), []byte("redirected"))
	assertResolvedBundle(t, resolver, testAuthoritySourceRef, secondSource, secondProof)

	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	if authority.Service() != nil {
		t.Fatal("closed authority still exposed its policy service")
	}
	if _, _, err := resolver.Resolve(context.Background(), testAuthoritySourceRef, testAuthorityPlanDigest); err == nil ||
		!strings.Contains(err.Error(), "resolver is closed") {
		t.Fatalf("closed resolver error = %v", err)
	}
	if err := authority.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestAuthorityBundleResolverSelectsOnlyConfiguredSourceAndExactPlan(t *testing.T) {
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	writeAuthorityBundle(t, firstDirectory, testAuthorityPlanDigest, []byte("first"), []byte("first-proof"))
	writeAuthorityBundle(t, secondDirectory, testAuthorityPlanDigest, []byte("second"), []byte("second-proof"))

	first := testAuthorityConfiguration(firstDirectory)[0]
	second := testAuthorityConfiguration(secondDirectory)[0]
	second.SourceRef = "authority:second"
	second.AuthorizerRef = "identity:second-owner"
	authority, err := OpenAuthority([]AuthoritySource{first, second}, &discardAuthorityLedger{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = authority.Close() })
	assertResolvedBundle(t, authority.resolver, first.SourceRef, []byte("first"), []byte("first-proof"))
	assertResolvedBundle(t, authority.resolver, second.SourceRef, []byte("second"), []byte("second-proof"))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := authority.resolver.Resolve(cancelled, first.SourceRef, testAuthorityPlanDigest); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolve error = %v", err)
	}
	if _, _, err := authority.resolver.Resolve(context.Background(), "authority:unknown", testAuthorityPlanDigest); err == nil ||
		!strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unknown source error = %v", err)
	}
	for _, digest := range []string{
		"", "sha256:../bundle", "SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if _, _, err := authority.resolver.Resolve(context.Background(), first.SourceRef, digest); err == nil ||
			!strings.Contains(err.Error(), "canonical SHA-256") {
			t.Errorf("digest %q error = %v", digest, err)
		}
	}
}

func TestOpenAuthorityRejectsInvalidStartupConfiguration(t *testing.T) {
	directory := t.TempDir()
	regularPath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(regularPath, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(t.TempDir(), "directory-link")
	if err := os.Symlink(directory, symlinkPath); err != nil {
		t.Fatal(err)
	}
	valid := testAuthorityConfiguration(directory)[0]

	tests := map[string]struct {
		sources []AuthoritySource
		ledger  policy.ApprovalLedger
		want    string
	}{
		"absent source":        {sources: nil, ledger: &discardAuthorityLedger{}, want: "at least one source"},
		"empty source ref":     {sources: mutateSource(valid, func(source *AuthoritySource) { source.SourceRef = "" }), ledger: &discardAuthorityLedger{}, want: "trust root"},
		"empty authorizer":     {sources: mutateSource(valid, func(source *AuthoritySource) { source.AuthorizerRef = "" }), ledger: &discardAuthorityLedger{}, want: "trust root"},
		"invalid public key":   {sources: mutateSource(valid, func(source *AuthoritySource) { source.PublicKey = source.PublicKey[:31] }), ledger: &discardAuthorityLedger{}, want: "Ed25519 public key"},
		"relative directory":   {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory = "bundles" }), ledger: &discardAuthorityLedger{}, want: "absolute path"},
		"unclean directory":    {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory += string(os.PathSeparator) }), ledger: &discardAuthorityLedger{}, want: "path must be clean"},
		"NUL directory":        {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory += "\x00suffix" }), ledger: &discardAuthorityLedger{}, want: "absolute path"},
		"regular file":         {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory = regularPath }), ledger: &discardAuthorityLedger{}, want: "not a direct directory"},
		"symlink directory":    {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory = symlinkPath }), ledger: &discardAuthorityLedger{}, want: "not a direct directory"},
		"missing directory":    {sources: mutateSource(valid, func(source *AuthoritySource) { source.BundleDirectory = filepath.Join(t.TempDir(), "missing") }), ledger: &discardAuthorityLedger{}, want: "inspect directory"},
		"duplicate source ref": {sources: []AuthoritySource{valid, valid}, ledger: &discardAuthorityLedger{}, want: "duplicates source reference"},
		"absent ledger":        {sources: []AuthoritySource{valid}, ledger: nil, want: "requires a resolver, ledger, and clock"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			authority, err := OpenAuthority(test.sources, test.ledger)
			if authority != nil {
				_ = authority.Close()
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("OpenAuthority error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDecodeAuthorityBundleRequiresStrictExactEnvelope(t *testing.T) {
	valid := encodedAuthorityBundle{
		SchemaVersion: AuthorityBundleSchemaVersion,
		Source:        base64.RawURLEncoding.EncodeToString([]byte("source")),
		Proof:         base64.RawURLEncoding.EncodeToString([]byte("proof")),
	}
	validBytes := encodeAuthorityBundleForTest(t, valid)
	source, proof, err := decodeAuthorityBundle(validBytes)
	if err != nil || !bytes.Equal(source, []byte("source")) || !bytes.Equal(proof, []byte("proof")) {
		t.Fatalf("decode valid bundle = %q, %q, %v", source, proof, err)
	}

	unknown := map[string]any{
		"schema_version": AuthorityBundleSchemaVersion,
		"source":         valid.Source, "proof": valid.Proof, "extra": true,
	}
	unknownBytes, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		contents []byte
		want     string
	}{
		"empty":          {nil, "empty or exceeds"},
		"file oversized": {bytes.Repeat([]byte{' '}, maximumAuthorityBundleBytes+1), "empty or exceeds"},
		"malformed":      {[]byte(`{"schema_version":`), "strict I-JSON"},
		"duplicate": {[]byte(`{"schema_version":"sworn-authority-bundle-v1","source":"c291cmNl","source":"c291cmNl","proof":"cHJvb2Y"}`),
			"duplicate object name"},
		"unknown":       {unknownBytes, "unknown field"},
		"missing":       {[]byte(`{"schema_version":"sworn-authority-bundle-v1","source":"c291cmNl"}`), "missing field"},
		"wrong schema":  {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: "future", Source: valid.Source, Proof: valid.Proof}), "unknown authority bundle schema"},
		"wrong type":    {[]byte(`{"schema_version":"sworn-authority-bundle-v1","source":1,"proof":"cHJvb2Y"}`), "decode authority bundle"},
		"padded source": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: AuthorityBundleSchemaVersion, Source: "c291cmNl==", Proof: valid.Proof}), "canonical base64url"},
		"source newline": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: AuthorityBundleSchemaVersion, Source: valid.Source + "\n", Proof: valid.Proof}),
			"canonical base64url"},
		"invalid alphabet": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: AuthorityBundleSchemaVersion, Source: "++//", Proof: valid.Proof}),
			"canonical base64url"},
		"empty source": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: AuthorityBundleSchemaVersion, Source: "", Proof: valid.Proof}), "source is empty or exceeds"},
		"empty proof":  {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{SchemaVersion: AuthorityBundleSchemaVersion, Source: valid.Source, Proof: ""}), "proof is empty or exceeds"},
		"source oversized": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{
			SchemaVersion: AuthorityBundleSchemaVersion,
			Source:        base64.RawURLEncoding.EncodeToString(make([]byte, policy.MaximumAuthoritySourceBytes+1)),
			Proof:         valid.Proof,
		}), "source is empty or exceeds"},
		"proof oversized": {encodeAuthorityBundleForTest(t, encodedAuthorityBundle{
			SchemaVersion: AuthorityBundleSchemaVersion,
			Source:        valid.Source,
			Proof:         base64.RawURLEncoding.EncodeToString(make([]byte, policy.MaximumAuthorityProofBytes+1)),
		}), "proof is empty or exceeds"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeAuthorityBundle(test.contents); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeAuthorityBundle error = %v, want %q", err, test.want)
			}
		})
	}

	maximum := encodedAuthorityBundle{
		SchemaVersion: AuthorityBundleSchemaVersion,
		Source:        base64.RawURLEncoding.EncodeToString(make([]byte, policy.MaximumAuthoritySourceBytes)),
		Proof:         base64.RawURLEncoding.EncodeToString(make([]byte, policy.MaximumAuthorityProofBytes)),
	}
	maximumBytes := encodeAuthorityBundleForTest(t, maximum)
	if len(maximumBytes) > maximumAuthorityBundleBytes {
		t.Fatalf("valid maximum bundle length %d exceeds file ceiling %d", len(maximumBytes), maximumAuthorityBundleBytes)
	}
	if source, proof, err := decodeAuthorityBundle(maximumBytes); err != nil ||
		len(source) != policy.MaximumAuthoritySourceBytes || len(proof) != policy.MaximumAuthorityProofBytes {
		t.Fatalf("maximum bundle = source %d, proof %d, %v", len(source), len(proof), err)
	}
}

func TestAuthorityBundleFilenameUsesOnlyCanonicalDigestHex(t *testing.T) {
	filename, err := authorityBundleFilename(testAuthorityPlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimPrefix(testAuthorityPlanDigest, "sha256:") + ".json"; filename != want {
		t.Fatalf("bundle filename = %q, want %q", filename, want)
	}
	if strings.ContainsAny(filename, `/\\:`) {
		t.Fatalf("bundle filename contains a path separator or scheme delimiter: %q", filename)
	}
}

func testAuthorityConfiguration(directory string) []AuthoritySource {
	key := make(ed25519.PublicKey, ed25519.PublicKeySize)
	for index := range key {
		key[index] = byte(index + 1)
	}
	return []AuthoritySource{{
		SourceRef: testAuthoritySourceRef, AuthorizerRef: testAuthorityAuthorizerRef,
		PublicKey: key, BundleDirectory: directory,
	}}
}

func mutateSource(source AuthoritySource, mutate func(*AuthoritySource)) []AuthoritySource {
	mutate(&source)
	return []AuthoritySource{source}
}

func assertResolvedBundle(
	t *testing.T,
	resolver *authorityBundleResolver,
	sourceRef string,
	wantSource []byte,
	wantProof []byte,
) {
	t.Helper()
	source, proof, err := resolver.Resolve(context.Background(), sourceRef, testAuthorityPlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, wantSource) || !bytes.Equal(proof, wantProof) {
		t.Fatalf("resolved bundle = %q, %q; want %q, %q", source, proof, wantSource, wantProof)
	}
}

func writeAuthorityBundle(t *testing.T, directory, digest string, source, proof []byte) {
	t.Helper()
	filename, err := authorityBundleFilename(digest)
	if err != nil {
		t.Fatal(err)
	}
	contents := encodeAuthorityBundleForTest(t, encodedAuthorityBundle{
		SchemaVersion: AuthorityBundleSchemaVersion,
		Source:        base64.RawURLEncoding.EncodeToString(source),
		Proof:         base64.RawURLEncoding.EncodeToString(proof),
	})
	temporary, err := os.CreateTemp(directory, ".authority-bundle-*")
	if err != nil {
		t.Fatal(err)
	}
	temporaryName := temporary.Name()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporaryName, filepath.Join(directory, filename)); err != nil {
		t.Fatal(err)
	}
}

func encodeAuthorityBundleForTest(t *testing.T, bundle encodedAuthorityBundle) []byte {
	t.Helper()
	contents, err := protocol.EncodeCanonical(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
