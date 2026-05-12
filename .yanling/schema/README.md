# Machine-First Schema Standard

This directory defines the canonical machine-first export schema for Go modules.

Version:
- `yanling.machine-first/v1`

Schema files:
- `yanling.machine-first.v1/module.schema.json`
- `yanling.machine-first.v1/symbols.schema.json`
- `yanling.machine-first.v1/package.schema.json`

Conventions:
- A generator should produce three artifacts under `.yanling/`:
  - `module.json`
  - `symbols.json`
  - `packages/*.json`
- All artifacts should set `schema_version = "yanling.machine-first/v1"`.
- Producers may add extra fields, but consumers should rely on fields defined in these schemas.

Validation suggestion:
- Validate each generated JSON file against the matching schema before publishing.
