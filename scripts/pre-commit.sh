#!/bin/bash
# Pre-commit hook para vault-graph.
#
# Corre antes de cada commit:
#   1. go build ./...    — compila todo el proyecto.
#   2. go vet ./...      — chequeos estáticos.
#   3. go test ./...     — corre los tests (puede tomar 30-60s).
#
# Si algo falla, aborta el commit con exit 1.
# Para saltarse el hook (NO recomendado): git commit --no-verify
#
# Instalación: ver scripts/install-hooks.sh
# O manualmente: cp scripts/pre-commit.sh .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit

set -e

# Mensaje breve al usuario
echo "→ pre-commit: corriendo build, vet, test..."

# 1. Build
echo "  [1/3] go build ./..."
if ! go build ./...; then
    echo "❌ go build falló. Commit abortado."
    exit 1
fi

# 2. Vet
echo "  [2/3] go vet ./..."
if ! go vet ./...; then
    echo "❌ go vet encontró issues. Commit abortado."
    exit 1
fi

# 3. Tests
echo "  [3/3] go test ./..."
if ! go test ./...; then
    echo "❌ go test falló. Commit abortado."
    exit 1
fi

echo "✅ pre-commit: build + vet + test pasaron."
exit 0
