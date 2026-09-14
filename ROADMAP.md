# Roadmap — vault-graph

## Estado actual

- Estado: activo
- Último hito: 2026-09-13 — backend dual creado en `dev-store-backend-dual`
- Próxima acción: mergear backend dual a `master`, validar con datos reales

## Fase 0 — Base operativa

- [x] Ficha del proyecto en el vault.
- [x] Documentación mínima del repo.
- [x] Validación local documentada (`go test ./...`).
- [x] `.gitignore` protege secretos y artefactos generados.

## Fase 1 — Funcionalidad inicial

- [x] Sincronización de vault al grafo (`vg sync`).
- [x] Comandos de consulta: neighbors, backlinks, path.
- [x] Detección de huérfanos y enlaces rotos (`orphans`, `broken`).
- [ ] Backend PostgreSQL/AGE validado con volumen ≥10k notas.
- [ ] Dashboard/endpoint web para consultas interactivas (opcional).

## Bloqueos

- Sin bloqueos conocidos.

