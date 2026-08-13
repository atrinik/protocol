# Contributing

Direct human-written code and specification contributions are welcome. They
follow the same maintainer review, provenance, licensing, testing, and
validation requirements as changes developed through agentic workflows.

Use Conventional Commits for commits and pull-request titles. New wire fields
must identify the authoritative schema/specification, producers, consumers,
bounds, state/authorization validation, failure behavior, compatibility
decision, and language-neutral fixtures.

Before implementation, confirm that inputs comply with
[PROVENANCE.md](PROVENANCE.md). Independent implementation remains the default
when exact historical reuse cannot be proven. Each selected, independently
separable contribution must itself fit one applicable canonical registry row's
“original past Atrinik contributions solely authored” scope. Historical rows
cannot be combined to cover a jointly authored contribution, agent-generated
output, or inseparable mixed work; later or agent-generated material needs its
own contemporaneous compatible rights. Complete history, identity,
embedded-material, separability, transformation, reviewer, and destination
evidence is required. No class of tests, fixtures, generated bindings, assets,
or dependency code is presumed covered. This source-reuse route does not
permit GPL/AGPL dependencies or bundles, and the Classic source stays
GPL-2.0-or-later. Record every copied or generated fixture and every reuse
candidate in the reuse manifest; unknown or mixed provenance fails closed.
Pull requests must state whether generated files changed and whether packaged
data or assets retain separate terms.

Run:

```sh
tools/generate.sh
tools/validate.sh
```

Review generated diffs; never edit generated Go/Rust contracts directly.
