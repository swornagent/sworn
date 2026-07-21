# Baton conformance suite

Run Baton's portable checks with:

```sh
python3 conformance/check.py
```

The script requires Python 3 and `jsonschema`. It runs:

- strict I-JSON rejection cases, including duplicate keys, unsafe numbers, and
  invalid Unicode;
- RFC 8785 canonicalization vectors;
- positive and negative Draft 2020-12 schema fixtures with `format` assertions;
  and
- executable cross-record mutations over the complete example chain.

The last group deterministically exercises digest, authority, policy,
candidate-tree, evidence, verifier-dispatch, identity, timestamp, and board
bindings in the portable record model. It is deliberately more than independent
schema checks, but it does not prove a real resolver, store, Git boundary,
sandbox, or process boundary.

`manifest.json` also publishes real-boundary engine cases. This local script
reports those as **NOT RUN** because they require an actual engine, Git object
database, persistence store, subprocess executor, sandbox, and crash injection.
An implementation must run those cases through its real binary before claiming
Baton engine conformance.
