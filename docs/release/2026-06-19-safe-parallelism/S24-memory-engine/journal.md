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
## 2026-06-21 Implementer Session

**State transition:** `design_review` → `in_progress` → `implemented`

**Decisions & Trade-offs:**
- Addressed all 6 Coach pins from design review inline.
- Implemented `Embedder` interface and drivers for Voyage, OAI-compat, and Ollama.
- Implemented SQLite vector index with change detection via SHA256 and cosine similarity.
- Implemented entry discovery for Claude Code (`MEMORY.md`), flat files, and custom paths.
- Added `sworn memory build` command with `--force` flag.
- Used `modernc.org/sqlite` for the vector index.
- Stored embeddings as `[]float32` in little-endian IEEE 754 binary format.
- Added tests for embedding drivers, vector index, and entry discovery.
- Generated `proof.md` and passed first-pass verification.

**Subagent dispatches:**
- None (runtime does not support subagent dispatch).

**Skeptic panel:**
- skipped — runtime does not support subagent dispatch.
