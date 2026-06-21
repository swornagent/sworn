# Journal — S24-memory-engine

## Init — 2026-06-29

- **State**: `planned` → `design_review`
- **Actions**: Produced design TL;DR with 6 mandatory sections.
- **Decisions**:
  - SHA256 of (path + content) for entry identity; for flat-file splits, path includes an index fragment
  - Embedding batch order MUST be preserved across split requests
  - API keys read at call time (not config load time)
  - SQLite connection opens per build, closes on completion
- **Open questions**: raised 3 in §6 of design.md (BLOB format, modernc driver path, MEMORY.md format stability)
- **Next**: await Captain review + Coach ack/decline