#!/usr/bin/env bash
# test-vg-lock.sh — prueba que el wrapper scripts/vg serializa el acceso a Kuzu.
#
# Uso: scripts/test-vg-lock.sh [ruta-del-binario-vault-graph]
#
# Monta un vault de prueba en un directorio temporal y lanza comandos en
# paralelo contra un `sync` en curso. El caso 0 usa el binario SIN wrapper y
# exige que falle: si no falla, los casos siguientes no prueban nada.

set -uo pipefail

AQUI="$(cd "$(dirname "$0")" && pwd)"
WRAPPER="$AQUI/vg"
BIN="${1:-$HOME/.local/bin/vault-graph}"
[ -x "$BIN" ] || { echo "✖ no hay binario ejecutable en $BIN" >&2; exit 2; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/vault"
for i in $(seq 1 400); do echo "[[n$((i + 1))]]" > "$TMP/vault/n$i.md"; done

export VAULT_PATH="$TMP/vault" KUZU_PATH="$TMP/k.kuzu" VG_BACKEND=kuzu
export VAULT_GRAPH_BIN="$BIN" VAULT_GRAPH_CONFIG="$TMP/no-existe.env" VG_LOCK_FILE="$TMP/vg.lock"

fallos=0
caso() { # $1 = nombre  $2 = exit esperado  $3 = exit obtenido
    if [ "$2" = "$3" ]; then
        echo "✔ $1 (exit $3)"
    else
        echo "✖ $1: se esperaba exit $2 y dio $3"
        fallos=$((fallos + 1))
    fi
}

# Caso 0 — control: sin wrapper, el segundo proceso choca.
"$BIN" sync >/dev/null 2>&1 &
sleep 1
"$BIN" stats >/dev/null 2>&1
caso "control: binario directo durante un sync falla" 1 $?
wait

# Caso 1 — lectura durante un sync, por el wrapper: espera y pasa.
"$WRAPPER" sync >/dev/null 2>&1 &
sleep 1
"$WRAPPER" stats >/dev/null 2>&1
caso "wrapper: stats durante un sync" 0 $?
wait

# Caso 2 — dos sync a la vez por el wrapper: pasan los dos.
"$WRAPPER" sync >/dev/null 2>&1 &
primero=$!
sleep 1
"$WRAPPER" sync >/dev/null 2>&1
caso "wrapper: segundo sync durante un sync" 0 $?
wait "$primero"
caso "wrapper: primer sync" 0 $?

# Caso 3 — espera agotada: sale con 75 y no con el error de Kuzu.
"$WRAPPER" sync >/dev/null 2>&1 &
sleep 1
VG_LOCK_WAIT=0 "$WRAPPER" stats >/dev/null 2>&1
caso "wrapper: espera agotada" 75 $?
wait

# Caso 4 — el código de salida del binario se conserva.
"$WRAPPER" subcomando-inexistente >/dev/null 2>&1
caso "wrapper: conserva el exit del binario" 1 $?

echo ""
if [ "$fallos" -eq 0 ]; then
    echo "✔ test-vg-lock: todos los casos pasan."
    exit 0
fi
echo "✖ test-vg-lock: $fallos caso(s) fallaron."
exit 1
