# Gooo bounded observational equivalence projector

This repository proves or refutes bounded observational equivalence between a
`.gooo` semantic scenario and a generated Go 1.27 program. It makes no claim
about general program equivalence: only the declared scenario inputs,
observable state, ordered event trace, allowed effect vocabulary, bounds, and
normalization policy are compared.

The denominator is fixed before implementation in
[`contracts/denominator-v1.json`](contracts/denominator-v1.json). It contains
exactly twelve named invariants: four each under FOUNDATION, COHERENCE, and
REGRESSION, with four DRIVER, four OUTCOME, and four GUARDRAIL vector cells.
No scalar score or percentage is produced.

The implementation is verified only by GitHub Actions. Runtime outputs are
written to caller-owned output or temporary paths; the product runtime never
writes the repository, executes local tests, or requires a cross-project gate.
