package gitx

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// #381: a repository source may be larger than a Protocol record. Sources are
// read under their own cap; records keep MaxFileBytes.
func TestReadSourceBlobsAdmitsSourcesLargerThanARecordWithinItsOwnCap(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	runTestGit(t, directory, nil, "init", "--quiet", "--initial-branch=main")
	large := bytes.Repeat([]byte("x"), MaxFileBytes+1)
	tooLarge := bytes.Repeat([]byte("y"), MaxSourceFileBytes+1)
	for name, body := range map[string][]byte{"large.go": large, "too_large.go": tooLarge} {
		if err := os.WriteFile(filepath.Join(directory, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runTestGit(t, directory, nil, "add", "--", "large.go", "too_large.go")
	runTestGit(t, directory, nil, "commit", "--quiet", "-m", "sources")
	repository, err := Open(directory, testGit)
	if err != nil {
		t.Fatal(err)
	}
	head, err := ParseOID(repository.ObjectFormat(), runTestGit(t, directory, nil, "rev-parse", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = repository.ReadBlobs(head, []string{"large.go"})
	requireGitxErrorCode(t, err, "RESOURCE_LIMIT")

	got, err := repository.ReadSourceBlobs(head, []string{"large.go"})
	if err != nil {
		t.Fatalf("ReadSourceBlobs(%d-byte source) = %v, want its bytes", len(large), err)
	}
	if !bytes.Equal(got["large.go"], large) {
		t.Fatalf("ReadSourceBlobs returned %d bytes, want %d", len(got["large.go"]), len(large))
	}

	_, err = repository.ReadSourceBlobs(head, []string{"too_large.go"})
	requireGitxErrorCode(t, err, "RESOURCE_LIMIT")
}
