# Changelog

Todas las notas importantes de este proyecto se documentan aquí.

El formato sigue Keep a Changelog y versionado SemVer cuando aplique.

## [Unreleased]

### Added

- Backend dual: soporte para PostgreSQL/AGE como alternativa a Kuzu embebido (`VG_BACKEND`).
- Comandos adicionales: `stats`, `enrich`.

### Changed

- Retirado `deploy/systemd/` (timer de sincronización): `vg sync` corre por disparador.

- El backend predeterminado permanece en `kuzu` hasta que AGE sea estable en uso productivo.

### Fixed

- Wrapper `vg` versionado en `scripts/vg` con candado `flock`: un `vg` lanzado durante un `vg sync` ya no falla con `failed to open database with status 1`, espera su turno.
- `go.sum` versionado: un clon limpio compila.

### Security

