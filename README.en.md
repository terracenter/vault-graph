# vault-graph

CLI (`vg`) + backend engine that syncs an Obsidian vault into a queryable note graph. Loads nodes and edges from the vault into Kuzu embedded (default) or PostgreSQL with AGE, exposes commands for exploring neighbors, backlinks, paths, orphan notes, broken links, and full-text search.

## Status

- Status: active
- Version: see `VERSION` or `CHANGELOG.md`
- Operational documentation: `Obsidian/07.Desarrollos/16.vault-graph/index.md`

## Usage

```bash
# Sync vault to graph (load/update nodes and edges)
vg sync

# Explore note connections
vg neighbors "<note-title>"
vg backlinks "<note-title>"

# Advanced queries
vg path "<source>" "<target>"       # path between two notes
vg orphans                          # notes without incoming links
vg broken                           # notes with broken references
vg query "search text"              # full-text search by title or content
vg enrich                           # enrich graph metadata
vg stats                            # graph statistics
```

### Configuration

Environment variables / `.env`:

| Variable | Required | Description |
|---|---|---|
| `VAULT_PATH` | yes | Path to the Obsidian vault |
| `VG_BACKEND` | no | `kuzu` (default) or `age` |
| `KUZU_PATH` | yes if backend=kuzu | Path to Kuzu storage |
| `DATABASE_URL` | yes if backend=age | PostgreSQL/AGE connection URL |
| `OLLAMA_URL` | no | Ollama endpoint (if using embedding) |

Global flag: `--format json|table` (default: `table`).

## Development

```bash
# Build
go build -o vg .

# Test
go test ./...

# Lint
golangci-lint run
```

Requirements: Go 1.25+, Kuzu C library (for `kuzu` backend), or PostgreSQL + AGE extension (for `age` backend).

## Documentation

- `README.md`: main documentation.
- `SECURITY.md`: security policy.
- `CONTRIBUTING.md`: contribution rules.
- `ROADMAP.md`: project status and direction.
- `CHANGELOG.md`: change history.

## Security

Do not store secrets, tokens, `.env`, credential logs or local databases in the repository.
