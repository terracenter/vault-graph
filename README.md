# vault-graph

CLI (`vg`) + motor backend que sincroniza un vault Obsidian a un grafo de notas consultable. Carga nodos y aristas del vault en Kuzu embebido (por defecto) o PostgreSQL con AGE, expone comandos para explorar vecinos, enlaces entrantes, rutas, nodos huérfanos, enlaces rotos y búsqueda de texto completo.

## Estado

- Estado: activo
- Versión: ver `VERSION` o `CHANGELOG.md`
- Documentación operativa: `Obsidian/07.Desarrollos/16.vault-graph/index.md`

## Uso

```bash
# Sincronizar vault al grafo (carga/actualiza nodos y aristas)
vg sync

# Explorar conexiones de una nota
vg neighbors "<título-de-nota>"
vg backlinks "<título-de-nota>"

# Consultas avanzadas
vg path "<origen>" "<destino>"     # ruta entre dos notas
vg orphans                         # notas sin enlaces entrantes
vg broken                          # notas con referencias rotas
vg query "buscar texto"            # búsqueda por título o contenido
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

## Desarrollo

```bash
# Build
go build -o vg .

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
