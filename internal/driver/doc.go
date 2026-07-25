// Package driver implements Sworn's role-neutral process boundary.
//
// The package deliberately has no lifecycle or Baton-record authority. It
// validates the portable Baton driver wire contract, resolves one explicit
// provider/model selection, invokes one bounded child, and can seal one
// invocation-bound submission for a caller that already holds authority.
// Applying that submission and deciding what happens next belong elsewhere.
package driver
