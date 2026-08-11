# Contributing

Direct human-written code and specification contributions are welcome. They
follow the same maintainer review, provenance, licensing, testing, and
validation requirements as changes developed through agentic workflows.

Use Conventional Commits for commits and pull-request titles. New wire fields
must identify the authoritative schema/specification, producers, consumers,
bounds, state/authorization validation, failure behavior, compatibility
decision, and language-neutral fixtures.

Before implementation, confirm that inputs comply with [PROVENANCE.md](PROVENANCE.md).
Do not consult classic implementation source as a coding template. Record any
copied/generated fixture or approved-grantor candidate in the reuse manifest;
unknown or mixed provenance fails closed. Pull requests must state whether
generated files changed and whether packaged data/assets retain separate terms.

Run:

```sh
tools/generate.sh
tools/validate.sh
```

Review generated diffs; never edit generated Go/Rust contracts directly.
