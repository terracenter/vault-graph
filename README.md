# vault-graph

CLI (`vault-graph`, invocado por el wrapper `vg`) + motor backend que sincroniza un vault Obsidian a un grafo de notas consultable. Carga nodos y aristas del vault en Kuzu embebido (por defecto) o PostgreSQL con AGE, expone comandos para explorar vecinos, enlaces entrantes, rutas, nodos huérfanos, enlaces rotos y queries Cypher de solo lectura. No indexa el contenido de las notas.

## Estado

- Estado: activo
- Versión: ver `VERSION` o `CHANGELOG.md`
- Documentación operativa: `Obsidian/07.Desarrollos/16.vault-graph/index.md`

## Uso

```bash
# Sincronizar vault al grafo (carga/actualiza nodos y aristas)
vg sync

# Explorar conexiones de una nota
vg neighbors "<ruta-relativa-al-vault>"
vg backlinks "<ruta-relativa-al-vault>"

# Consultas avanzadas
vg path "<origen>" "<destino>"     # ruta entre dos notas
vg orphans                         # notas sin enlaces entrantes
vg broken                          # notas con referencias rotas
vg query "<cypher>"               # query Cypher de solo lectura
vg enrich                          # enriquecer metadatos del grafo
vg stats                           # estadísticas del grafo
```

### Configuración

Variables de entorno / `.env`:

| Variable | Obligatorio | Descripción |
|---|---|---|
| `VAULT_PATH` | sí | Ruta al vault Obsidian |
| `VG_BACKEND` | no | `kuzu` (default) o `age` |
| `KUZU_PATH` | sí si backend=kuzu | Ruta al almacenamiento Kuzu |
| `DATABASE_URL` | sí si backend=age | URL de conexión PostgreSQL/AGE |
| `OLLAMA_URL` | no | Endpoint de Ollama (si se usa embedding) |

Flag global: `--format json|table` (default: `table`).

### Wrapper `vg`

`scripts/vg` es el punto de entrada de uso diario. Carga `~/.config/vault-graph/.env` y, con el
backend `kuzu`, toma un candado (`flock`) antes de llamar al binario: Kuzu embebido admite un solo
proceso a la vez, y sin el candado cualquier `vg` lanzado durante un `vg sync` falla con
`failed to open database with status 1`. Con el candado, el segundo proceso espera (hasta
`VG_LOCK_WAIT` segundos, default 120; agotada la espera sale con código 75).

```bash
go build -o ~/.local/bin/vault-graph .
install -m 755 scripts/vg ~/.local/bin/vg
scripts/test-vg-lock.sh            # prueba el candado contra un vault temporal
```

Todo lo que consulte el grafo debe pasar por `vg`, no por `vault-graph` directo.

## Desarrollo

```bash
# Build
go build -o vault-graph .

# Test
go test ./...

# Lint
golangci-lint run
```

Requisitos: Go 1.25+, Kuzu C library (para backend `kuzu`), o PostgreSQL + extension AGE (para backend `age`).

## Documentación

- `README.md`: documentación principal.
- `SECURITY.md`: política de seguridad.
- `CONTRIBUTING.md`: reglas de contribución.
- `ROADMAP.md`: estado y dirección del proyecto.
- `CHANGELOG.md`: historial de cambios.

## Seguridad

No guardar secretos, tokens, `.env`, logs con credenciales ni bases locales en el repo.
