# macOS temporary-path portability

macOS commonly exposes its temporary directory through `/var`, which resolves
to `/private/var`. Sworn's strict path admission correctly rejected that
non-canonical spelling, but `testing.T.TempDir` supplied it to positive fixtures.
The same alias also made Sworn's own driver-certification workspace fail before
credential resolution.

The correction is deliberately narrow:

- tests canonicalise only their process-owned temporary root;
- the driver factory canonicalises only the root it just created; and
- journal, operator-config, and external workspace admission remain unchanged.

A synthetic symlinked `TMPDIR` reproduces the macOS layout on Linux. The
portable macOS CI lane now runs source tests, vet, and a CGO-free build.
Autonomous execution and full live certification remain Linux-only.
