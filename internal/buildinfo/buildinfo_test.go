package buildinfo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONReportsProtocolPin(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Write(&output, true); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	var info Info
	if err := json.Unmarshal(output.Bytes(), &info); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if info.BatonSourceCommit == "" || !strings.HasPrefix(info.BatonSnapshotDigest, "sha256:") {
		t.Fatalf("incomplete protocol identity: %+v", info)
	}
}
