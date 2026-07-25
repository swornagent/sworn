package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func TestVerifyReportsExactAdmission(t *testing.T) {
	t.Parallel()

	var firstOut, firstErr bytes.Buffer
	if code := run([]string{"verify"}, &firstOut, &firstErr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, firstErr.String())
	}
	var got verification
	if err := json.Unmarshal(firstOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := verification{
		Schema: goldenSchema,
		Identity: baton.Identity{
			PackageVersion:       baton.PackageVersion,
			TagName:              baton.TagName,
			TagObject:            baton.TagObject,
			Commit:               baton.Commit,
			Tree:                 baton.Tree,
			ArchiveSHA256:        baton.ArchiveSHA256,
			SupportPackageSHA256: baton.SupportPackageSHA256,
			ManifestSHA256:       baton.ManifestSHA256,
			AssetCount:           baton.AssetCount,
			AssetBytes:           baton.AssetBytes,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verification = %#v, want %#v", got, want)
	}

	var secondOut, secondErr bytes.Buffer
	if code := run([]string{"verify"}, &secondOut, &secondErr); code != 0 {
		t.Fatalf("second run() = %d, stderr = %q", code, secondErr.String())
	}
	if !bytes.Equal(firstOut.Bytes(), secondOut.Bytes()) {
		t.Fatal("verification output is not deterministic")
	}
}

func TestVerifyRejectsEveryOtherInvocation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"verify", "again"}, {"generate"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || stderr.String() != "usage: batongolden verify\n" {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}
