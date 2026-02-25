# Public Packages

This directory is reserved for reusable libraries that can be imported by external projects.

Current backend business code remains under `internal/` to enforce encapsulation.

Rules:
- Place only stable, generic utilities in `pkg/`.
- Keep domain/repository/service code private in `internal/`.
