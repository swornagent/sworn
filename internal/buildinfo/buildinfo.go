// Package buildinfo describes this Sworn binary and its Baton protocol pin.
package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/swornagent/sworn/internal/protocol"
)

var (
	version = "0.2.0-dev"
	commit  = "unknown"
)

// Info is the stable machine-readable version surface.
type Info struct {
	Version             string `json:"version"`
	Commit              string `json:"commit"`
	BatonVersion        string `json:"baton_version"`
	BatonSourceCommit   string `json:"baton_source_commit"`
	BatonSnapshotDigest string `json:"baton_snapshot_digest"`
}

// Current verifies the embedded snapshot before reporting its identity.
func Current() (Info, error) {
	if err := protocol.VerifySnapshot(); err != nil {
		return Info{}, fmt.Errorf("verify Baton snapshot: %w", err)
	}
	digest, err := protocol.SnapshotDigest()
	if err != nil {
		return Info{}, err
	}
	return Info{
		Version:             version,
		Commit:              commit,
		BatonVersion:        protocol.BatonVersion,
		BatonSourceCommit:   protocol.BatonSourceCommit,
		BatonSnapshotDigest: "sha256:" + digest,
	}, nil
}

// Write renders version information without consulting the network or runtime
// configuration.
func Write(out io.Writer, asJSON bool) error {
	info, err := Current()
	if err != nil {
		return err
	}
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}
	_, err = fmt.Fprintf(out, "sworn %s (%s)\nBaton %s %s\nsnapshot %s\n",
		info.Version,
		info.Commit,
		info.BatonVersion,
		info.BatonSourceCommit,
		info.BatonSnapshotDigest,
	)
	return err
}
