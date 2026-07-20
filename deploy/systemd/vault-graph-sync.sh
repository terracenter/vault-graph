#!/usr/bin/env bash
# vault-graph-sync.sh — auto-sync incremental del grafo AGE
#
# Disparado por vault-graph-sync.timer cada 15 min. No hace git pull incondicional:
# si contabo está a mitad de una sesión trabajando en una rama dev-* (working tree
# sucio, o rama distinta de main), se omite el pull esta corrida para no interferir
# con el trabajo activo — igual se corre sync --since-mtime sobre lo que ya hay en
# disco, así las ediciones locales se capturan de inmediato.
set -e

VAULT_DIR="/home/freddy/Workspace/Obsidian"
GRAPH_DIR="/home/freddy/Workspace/Desarrollo/vault-graph"

cd "$VAULT_DIR"
BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "main" ] && [ -z "$(git status --porcelain)" ]; then
  git pull --rebase --quiet || echo "WARN: pull falló, se sincroniza igual con lo que hay en disco"
else
  echo "INFO: rama '$BRANCH' no es main o hay cambios sin commitear — se omite git pull esta corrida"
fi

cd "$GRAPH_DIR"
./vault-graph sync --since-mtime
