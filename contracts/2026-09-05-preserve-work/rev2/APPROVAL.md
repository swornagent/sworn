# External approval of recovery revision 2

Brad approved this exact proposal in the conversation on 2026-09-06 with
"yes! approved", after the exact plan link/digest and a plain-language
summary: retain the existing implementation as unverified repair input, fix
the retention defect and expected-stderr fixture, rerun checks and independent
verification, then continue preservation before roster/live switching.

- Release: 2026-09-05-preserve-work
- Revision: 2
- Plan SHA-256: a9f730f5a7e2835995ddfd98688e628b517ff4ed1f597847f14d8a50e207ab2b
- Approval reference: operator://2026-09-05-preserve-work/2
- Prior target: 3ddcfadeeffb1ddc4a071d1b638711d5b809bb98
- Target ref: refs/heads/release/2026-09-05-preserve-work

The approved plan bytes are unchanged, including their historical proposal
status wording. This document records the subsequent external approval;
Sworn's native plan action must still bind the actual prepared target and
contract-tree commit. Neither this approval nor the retained patch is a
candidate verification or permission to relax A1-A6.

Preparation adds only contracts, review/approval documentation and the inert
compressed recovery input. Product source is unchanged from the source tested
by the full sequential baseline run documented in ../BASELINE.md. That run
already performed product tests, host E2E, product race tests, vet and format
checks before preparation commits. Native pin/lint and recovery artifact
checks are repeated for this revision; corrected product code requires fresh
full host gates and independent verification.
