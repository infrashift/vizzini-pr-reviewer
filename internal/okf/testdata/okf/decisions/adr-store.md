---
id: "adr-0001-store"
title: "ADR-0001: Parameterized SQL only"
type: "ADR"
tags: ["adr", "persistence"]
x:
  paths: ["internal/store/**"]
  domain: "store"
---

# ADR-0001: Parameterized SQL only

**Decision:** every query in `internal/store` binds user input as parameters
(`$1`, `$2`). Building SQL with string concatenation is forbidden.
