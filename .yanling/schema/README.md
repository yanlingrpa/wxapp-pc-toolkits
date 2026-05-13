# Machine-First Schema Standard

This directory defines the canonical machine-first export schema for Go modules, designed to enable AI-friendly analysis, multi-module merging, and automated tooling integration.

## Version

- `yanling.machine-first/v1`

## Schema Files

### Core Symbol & Module Metadata

#### `module.schema.json`
**Purpose**: Root metadata for the entire module.

**Contents**:
- `schema_version`: Fixed value "yanling.machine-first/v1"
- `generated_at`: ISO 8601 timestamp of generation
- `module`: Module specifier (e.g., github.com/org/repo)
- `counts`: Aggregate symbol statistics (packages, structs, functions, methods, etc.)
- `files`: File references (symbol_index, symbol_index_lite, package_dir, topics)
- `packages`: Array of package entries with import paths, directories, and dependencies

**Output file**: `.yanling/module.json`

---

#### `symbols.schema.json`
**Purpose**: Comprehensive symbol index with full documentation and type references.

**Contents**:
- `schema_version`: Fixed value "yanling.machine-first/v1"
- `generated_at`: ISO 8601 timestamp
- `module`: Module specifier
- `counts`: Symbol statistics
- `symbols`: Array of fully documented symbols with:
  - Metadata: id, name, qualified_name, kind, package, import_path
  - Documentation: doc, signature
  - Source location: file, line, column
  - Structure details: fields, methods, params, results
  - Type references and dependencies

**Includes shared definitions**:
- `CountsDoc`: Symbol count breakdown
- `SourceDoc`: File location information
- `TypeRefDoc`: Type reference with import path and qualified name

**Output files**: 
- `.yanling/symbols.json` (complete)
- `.yanling/symbols.lite.json` (lightweight, IDE-friendly without full docs)

---

#### `package.schema.json`
**Purpose**: Detailed per-package documentation and symbol catalog.

**Contents** (one file per package in `packages/` directory):
- `schema_version`: Fixed value "yanling.machine-first/v1"
- `generated_at`: ISO 8601 timestamp
- `module`: Module specifier
- `package`: Package details including:
  - name, import_path, directory
  - doc, files, imports
  - dependencies, counts
  - symbols (full SymbolDoc entries with all details)

**Output files**: `.yanling/packages/{import_path_encoded}.json`

---

### Event & Topic Schema

#### `topics.schema.json`
**Purpose**: Catalog of published event topics extracted from `rt.Publish()` calls.

**Contents**:
- `schema_version`: Fixed value "yanling.machine-first/v1"
- `generated_at`: ISO 8601 timestamp
- `module`: Module specifier
- `topics`: Array of TopicDoc entries with:
  - `specifier`: Module identifier
  - `name`: Topic name (event identifier)
  - `go_struct_name`: Original Go struct name for the payload
  - `go_import_path`: Import path of the payload struct
  - `doc`: Topic documentation
  - `direction`: "publish" (fixed)
  - `payload`: JSON Schema describing the event payload structure

**Output file**: `.yanling/topics.json`

---

### Configuration Schema

#### `config.schema.json`
**Purpose**: Module configuration and runtime requirements (RPA script metadata).

**Contents**:
- `module`: ModuleInfo with metadata (specifier, package, description, tags, author, license, etc.)
- `gui_apps`: GUI application configurations (launcher, args, window settings)
- `web_apps`: Web/browser application configurations (URL, browser type, headless mode)
- `mobile_apps`: Mobile app configurations (package, activity, intent flags)
- `variables`: Global script variable definitions (type, default, required)
- `file_permissions`: Filesystem access permissions (URL patterns, access levels)
- `api_permissions`: Network API access permissions
- `worker_dependencies`: Worker module dependencies
- `script_dependencies`: Script module specifier imports

**Output file**: `.yanling/config.json`

---

### Index & Discovery

#### `index.schema.json` (Module Index)
**Purpose**: Lightweight, AI-friendly module index for multi-module discovery and merging.

**Contents**:
- `schema_version`: Fixed value "yanling.machine-first/v1"
- `generated_at`: ISO 8601 timestamp
- `module`: Module specifier
- `files`: File references (symbol_index, symbol_index_lite, package_dir, topics)
- `packages`: Mini package entries (name, import_path, directory, package_file)
- `topics`: Mini topic entries (name, specifier, go_struct_name, go_import_path, doc)
- `symbols`: Mini symbol entries (name, kind, import_path, package, doc, package_file)

**Use case**: Provides a single-file entry point for AI systems to:
- Discover module exports and topics
- Locate detailed documentation files
- Navigate multi-module merges without loading full symbol tables

**Output file**: `.yanling/index.json`

---

## Generation Workflow

### Recommended Producer Output

A generator should produce the following artifacts under `.yanling/`:

```
.yanling/
├── module.json              # Root module metadata
├── symbols.json             # Full symbol index
├── symbols.lite.json        # Lightweight symbol index (optional, IDE use)
├── topics.json              # Published event topics
├── index.json               # AI-friendly module index
├── config.json              # Configuration (if applicable)
├── packages/
│   ├── module__package1.json
│   ├── module__package2.json
│   └── ...
└── schema/
    └── yanling.machine-first.v1/
        ├── module.schema.json
        ├── symbols.schema.json
        ├── package.schema.json
        ├── topics.schema.json
        ├── config.schema.json
        └── index.schema.json
```

### Consumer Usage

**For Module Discovery**:
1. Load `index.json` to discover topics, packages, and key symbols
2. Load full `symbols.json` or specific `packages/*.json` for detailed analysis

**For Event Handling**:
1. Load `topics.json` to discover published events
2. Use `go_struct_name` and `go_import_path` to resolve payload types
3. Use `payload` (JSON Schema) for AI-driven code generation or validation

**For IDE Integration**:
1. Load `symbols.lite.json` for fast symbol navigation
2. Use `source` (file, line, column) to implement "Go to Definition"

**For Multi-Module Merging**:
1. Load all modules' `index.json` files
2. Deduplicate topics by specifier and name
3. Merge symbol tables, handling version conflicts

---

## Validation

### Recommended Practices

- **Producers**: Validate each generated JSON file against its schema before publishing
- **Consumers**: Validate index files on load to ensure consistency
- **Tooling**: Use JSON Schema validators (e.g., `ajv` for JavaScript, `jsonschema` for Python)

### Schema References

All output files should include a `$schema` field pointing to the appropriate schema:

```json
{
  "$schema": "./schema/yanling.machine-first.v1/module.schema.json",
  "schema_version": "yanling.machine-first/v1",
  ...
}
```

---

## Conventions & Best Practices

1. **Schema Version**: All artifacts must set `schema_version = "yanling.machine-first/v1"`.
2. **Timestamps**: Use ISO 8601 format (RFC 3339) for all `generated_at` fields.
3. **Import Paths**: Always use canonical Go import paths (e.g., `github.com/owner/repo/package`).
4. **Qualified Names**: Format as `{import_path}.{symbol_name}` for global disambiguation.
5. **Symbol IDs**: Format as `{import_path}#{symbol_name}` for persistent linking.
6. **Additional Fields**: Producers may add extra fields; consumers should ignore unknown fields (follow JSON Schema spec).
7. **Sorting**: Symbol arrays should be sorted by name for deterministic output and easier diffing.

---

## Examples

See the `yanling.machine-first.v1/` subdirectory for example files:
- `config.example.json`
- `topics.example.json`
