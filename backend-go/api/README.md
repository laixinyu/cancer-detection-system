# API Contracts

This directory stores API contract definitions for external and inter-service communication.

- OpenAPI: `../openapi.yaml`, `../openapi.json`
- Future gRPC proto files should be placed here (for generated stubs and schema governance).

Design intent:
- Keep transport contracts versioned and reviewable.
- Avoid embedding ad-hoc payload contracts directly in handlers.
