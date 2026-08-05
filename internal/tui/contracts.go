package tui

import (
	"context"

	"github.com/swornagent/sworn/internal/cockpit"
)

// Backend is the complete operator boundary used by the terminal UI. The UI
// projects these values and invokes admitted actions; it does not derive work
// or control the scheduler.
type Backend interface {
	Catalog(context.Context) (Catalog, error)
	Board(context.Context, Selection) (Board, error)
	Execute(context.Context, Selection, cockpit.Action, string) error
}

// Selection is the immutable identity carried through asynchronous board and
// action requests. Source is an opaque backend-owned snapshot identity.
type Selection struct {
	Release string
	RunID   string
	Source  string
}

// Catalog is one project snapshot.
type Catalog struct {
	Entries []CatalogEntry
}

// CatalogEntry is the plain-language release/run summary shown in the list.
type CatalogEntry struct {
	Selection Selection
	Status    string
	NeedsYou  string
	Checked   string
}

// Board is the complete presentation snapshot for one selected run. Summary
// fields are already plain language; the UI must not reinterpret run state.
type Board struct {
	Selection        Selection
	Graph            cockpit.Graph
	Actions          []cockpit.Action
	Attentions       []cockpit.AttentionView
	Diagnostics      []cockpit.Diagnostic
	Status           string
	What             string
	Next             string
	NeedsYou         string
	Checked          string
	CaptainAuthority string
	Stale            bool
	ThroughOffset    int64
}
