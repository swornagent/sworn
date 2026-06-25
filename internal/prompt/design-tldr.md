You are producing a **design TL;DR** — a concise pre-implementation design document
that captures the key decisions before any code is written. You are NOT implementing
anything; you are analysing the spec and describing the approach.

## Input

You receive a spec.md (the slice's specification). Read it carefully.

## Output format

Produce exactly six sections, each with a `##` heading. Do not add extra sections,
commentary, or markdown outside the six sections.

### §1 User-visible change

One sentence describing what the user will see or experience differently after this
slice lands. If the change has no direct user-visible surface (e.g. an internal
refactor), state what observable behaviour proves the change is live.

### §2 Design decisions not in the spec

At most 5 decisions you will need to make that the spec does not specify. Each is
one bullet (a single `-` line) naming the decision and the proposed resolution.
Focus on concrete choices: data structure, algorithm, package boundary, interface
shape, error handling posture, naming convention.

### §3 Files I'll touch by purpose

A bulleted list of files (paths from repo root) and the purpose of each change.
Group related files. Be specific: "Add a Generate function that reads spec.md,
calls the model, and writes design.md" rather than "Implement the feature."

### §4 Things I'm NOT doing

A bulleted list of work that might be expected but is explicitly out of scope
for this slice. Each item cites the section of the spec that rules it out.

### §5 Reachability plan

One sentence naming the integration point and the concrete user gesture or test
command that proves the change is live. E.g.: "Run `sworn run --task '...'`
and observe design.md written before any implementation commit."

### §6 Open questions

At most 3 questions you need answered before (or early in) implementation.
Each is one bullet. If you have no open questions, write "None."