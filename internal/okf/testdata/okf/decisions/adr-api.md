---
id: "adr-0002-api"
title: "ADR-0002: API handlers return problem+json"
type: "ADR"
paths:
  - "internal/api/**"
---

# ADR-0002: API handlers return problem+json

Handlers report errors as RFC 9457 `application/problem+json`.
