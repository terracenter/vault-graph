# vault-graph

CLI (`vault-graph`, invoked through the `vg` wrapper) + backend engine that syncs an Obsidian vault into a queryable note graph. Loads nodes and edges from the vault into Kuzu embedded (default) or PostgreSQL with AGE, exposes commands for exploring neighbors, backlinks, paths, orphan notes, broken links, and read-only Cypher queries. It does not index note contents.

## Status

- Status: active
- Version: see `VERSION` or `CHANGELOG.md`
- Operational documentation: `Obsidian/07.Desarrollos/16.vault-graph/index.md`

## Usage

```bash
# Sync vault to graph (load/update nodes and edges)
vg sync

# Explore note connections
vg neighbors "<path-relative-to-vault>"
vg backlinks "<path-relative-to-vault>"

# Advanced queries
vg path "<source>" "<target>"       # path between two notes
vg orphans                          # notes without incoming links
vg broken                           # notes with broken references
vg query "<cypher>"                # read-only Cypher query
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

### `vg` wrapper

`scripts/vg` is the day-to-day entry point. It loads `~/.config/vault-graph/.env` and, with the
`kuzu` backend, takes a lock (`flock`) before calling the binary: embedded Kuzu allows a single
process at a time, and without the lock any `vg` started during a `vg sync` fails with
`failed to open database with status 1`. With the lock, the second process waits (up to
`VG_LOCK_WAIT` seconds, default 120; on timeout it exits with code 75).

```bash
go build -o ~/.local/bin/vault-graph .
install -m 755 scripts/vg ~/.local/bin/vg
scripts/test-vg-lock.sh            # tests the lock against a temporary vault
```

Anything that queries the graph must go through `vg`, not `vault-graph` directly.

## Development

```bash
# Build
go build -o vault-graph .

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
