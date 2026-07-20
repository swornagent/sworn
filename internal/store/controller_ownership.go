package store

import "errors"

var (
	// ErrControllerOwnershipUnavailable means another controller currently
	// owns either the retained control Store or its containing namespace.
	ErrControllerOwnershipUnavailable = errors.New("controller ownership is unavailable")

	// ErrInvalidControllerOwnership means an ownership handle is nil, copied,
	// foreign, stale, released, in the wrong phase, or no longer names the
	// exact Store identity retained at open time.
	ErrInvalidControllerOwnership = errors.New("invalid controller ownership")

	// ErrControllerOwnershipUnsupported means this operating system cannot
	// provide Sworn's required crash-released process-shared locks.
	ErrControllerOwnershipUnsupported = errors.New("controller ownership is unsupported")
)
