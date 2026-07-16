# Fail-closed rollback checks

The proof checker derives the envelope before comparing object identities. These
negative runs demonstrate that it refuses to narrow the boundary or substitute a
later release-record checkpoint for the implementation head.

```text
$ proof/check-rollback.sh --head 640396fa8cc319229d6f96dedfdbef65dbe317fe
ROLLBACK_CHECK FAIL: rollback must contain exactly one post-frozen ordinary semantic restoration commit
exit: 1

$ proof/check-rollback.sh --head b3ef0c2
ROLLBACK_CHECK FAIL: unexpected later ordinary authority 4b38887e666f7e4ab664bac4780535b080ad54eb; only the pinned implementation head may restore semantics
exit: 1
```

The positive inventory is in `complete-rollback-envelope.txt`. The checker also
fails on any non-parent-two-exact semantic merge contribution, authored/merge
overlap, baseline mode/blob/absence mismatch, S02 record rewrite, or premature
S20 state transition. A successor S20 state is permitted only when the live S19
record has `state: verified`, the same pinned implementation head, a `pass`
verdict, a fresh-context flag, and a verdict timestamp; otherwise it fails
closed as premature.
