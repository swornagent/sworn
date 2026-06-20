Design is clear and well-scoped. 2 critical apply-inline fixes + 1 convention note + 3 Coach decisions logged (§6). Coach's answers to §6 follow in this ack.

1. **JSON struct tags.** Add `json:"token"`, `json:"email"`, `json:"tier"`, `json:"expires_at"` to the Credentials struct. Apply inline. AC3 failure without this.
2. **Logout no-op.** After `os.Remove`, suppress `errors.Is(err, os.ErrNotExist)`. One line. Apply inline. AC4 failure without this.
3. **Shared main.go.** T3's additions to `cmd/sworn/main.go` are additive dispatch only (`case "login":` / `case "logout":` / `case "account":`). No structural changes. T4 resolves merge hunks on assembly. Acknowledge in §3 notes.
4. **Auth endpoint (§6 Q1).** Use `SWORN_AUTH_URL` env var with a compile-time default baked in via ldflags (e.g. `https://auth.sworn.sh/device`). DeviceCodeFlow already accepts an `authEndpoint` param — tests use that; production uses `os.Getenv("SWORN_AUTH_URL")` with the ldflags fallback when unset.
5. **Tier format (§6 Q3).** Tier is a free-text string from the server (`"free"`, `"pro"`, etc.). Use `Tier string` in the Credentials struct. No client-side enum. Future tiers need no client change.
6. **Permissions warning (§6 Q2).** Enforce silently: write `credentials.json` with `os.FileMode(0600)` — no warning, no Load() check. The right permissions are always set at write time.

Address pins 1–6 inline during implementation, then proceed to in_progress.
