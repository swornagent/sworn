# macOS temporary-path portability

macOS commonly exposes its temporary directory through `/var`, which resolves
to `/private/var`. Sworn's strict path admission correctly rejected that
non-canonical spelling, but `testing.T.TempDir` supplied it to positive fixtures.
The same alias also made Sworn's own driver-certification workspace fail before
credential resolution.

The correction is deliberately narrow:

- tests canonicalise only their process-owned temporary root;
- the driver factory canonicalises only the root it just created; and
- exact-source merge attributes use an isolated index on Apple Git;
- explicit product bases use deterministic private merge heads;
- golden Baton checks receive one canonical Git and temporary root; and
- journal, operator-config, and external workspace admission remain unchanged.

A synthetic symlinked `TMPDIR` reproduces the macOS layout on Linux. The
portable macOS CI lane runs the complete package surface serially, then vet and
a CGO-free build; serial packages keep subprocess-heavy Git suites independent.
Autonomous execution and full live certification remain Linux-only.
