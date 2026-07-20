//go:build linux

package control

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/swornagent/sworn/internal/store"
)

func TestClaimedCheckTerminationReleasesOwnershipForRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, err := store.Open(ctx, filepath.Join(root, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	ownerID := "check-controller-crashed"
	ownership, err := journal.AcquireControllerOwnership(ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Activate(ctx, journal, ownerID); err != nil {
		t.Fatal(err)
	}
	var calls []string
	controller := &Controller{
		ownership: ownership, ownerID: ownerID, journal: journal,
		checks: CheckService{
			journal: &checkServiceJournalFixture{calls: &calls},
			worker:  &checkServiceWorkerFixture{calls: &calls, mode: "goexit"},
		},
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer func() { _ = recover() }()
		_ = controller.executeClaimedCheck(ctx, store.AuthorizedCheckLease{})
		runtime.Goexit()
	}()
	<-finished
	if !controller.closed {
		t.Fatal("terminated check controller remained open")
	}
	successor, err := journal.AcquireControllerOwnership("check-controller-restarted")
	if err != nil {
		t.Fatalf("restart could not acquire released check ownership: %v", err)
	}
	if err := successor.Activate(ctx, journal, "check-controller-restarted"); err != nil {
		t.Fatalf("restart could not activate after recovery barrier: %v", err)
	}
	if err := successor.Close(); err != nil {
		t.Fatal(err)
	}
}
