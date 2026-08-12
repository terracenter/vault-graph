#!/bin/bash
# Instala el pre-commit hook para vault-graph.
#
# Hace ejecutable scripts/pre-commit.sh y lo copia a .git/hooks/pre-commit.
# Idempotente: si ya está instalado, solo verifica y reporta.
#
# Uso: ./scripts/install-hooks.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SOURCE="$SCRIPT_DIR/pre-commit.sh"
TARGET="$REPO_ROOT/.git/hooks/pre-commit"

if [ ! -f "$SOURCE" ]; then
    echo "ERROR: no se encontró $SOURCE"
    exit 1
fi

# Verificar que estamos en un repo git
if [ ! -d "$REPO_ROOT/.git" ]; then
    echo "ERROR: $REPO_ROOT no es un repo git"
    exit 1
fi

# Si ya hay un pre-commit distinto, hacer backup
if [ -f "$TARGET" ] && [ ! "$TARGET" -ef "$SOURCE" ]; then
    BACKUP="$TARGET.bak.$(date +%s)"
    echo "→ existe pre-commit previo, moviendo a $BACKUP"
    mv "$TARGET" "$BACKUP"
fi

# Copiar y hacer ejecutable
cp "$SOURCE" "$TARGET"
chmod +x "$TARGET"

echo "✅ pre-commit hook instalado: $TARGET"
echo "   (para saltarlo: git commit --no-verify)"
echo "   (para desinstalar: rm $TARGET)"
