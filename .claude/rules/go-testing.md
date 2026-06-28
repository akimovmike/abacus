---
paths:
  - "**/*_test.go"
---

# Test conventions

- Strict TDD: write the failing test first. Table-driven where it fits.
- Integration tests carry `//go:build integration` and need bd/br binaries; default `make test` runs `-short`.
- Use `beads.MockClient` for backend deps; fixtures live in `testdata/`.
- UI golden snapshots: refresh only on purpose with `-update-golden`.
- Respect test size limits (file <= 800 lines, func <= 80 lines).
