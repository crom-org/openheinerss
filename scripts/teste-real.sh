#!/usr/bin/env bash
set -u

# Exercícios reais, deliberadamente curtos e sequenciais. Este arquivo não é
# descoberto pelo go test; execute-o somente quando houver cota/login válidos.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BIN=${OPENHEINERSS_BIN:-$ROOT/.tmp-openheinerss}
TIMEOUT=${OPENHEINERSS_TIMEOUT:-120s}

if [[ ! -x "$BIN" ]]; then
  (cd "$ROOT" && go build -o "$BIN" ./cmd/openheinerss)
fi

echo "== retomada Codex =="
"$BIN" harness test codex --retomar --timeout "$TIMEOUT" || true
echo "== retomada Claude conta2 =="
"$BIN" harness test claude-conta2 --retomar --timeout "$TIMEOUT" || true
echo "== Claude conta2 SDK =="
"$BIN" harness test claude-conta2 --modo sdk --timeout "$TIMEOUT" || true

PROMPT=$(mktemp)
TMP=$(mktemp -d)
trap 'rm -f "$PROMPT"; rm -rf "$TMP"' EXIT
cat >"$PROMPT" <<'EOF'
Crie o arquivo REAL-OK.txt contendo apenas OK e faça um commit git com essa alteração. Não faça mais nada.
EOF

git -C "$TMP" init -q -b main
git -C "$TMP" config user.email teste@localhost
git -C "$TMP" config user.name openheinerss-teste
git -C "$TMP" commit --allow-empty -qm inicio
mkdir -p "$TMP/agentes"
mkdir -p "$TMP/.openheinerss/harnesses"
cp "$ROOT/.openheinerss/harnesses/opencode-gratis.yaml" "$TMP/.openheinerss/harnesses/"
echo "== rodar Codex em repositório temporário =="
(cd "$TMP" && "$BIN" rodar teste-codex codex --prompt "$PROMPT" --pasta-agentes "$TMP/agentes" --tentativas 1) || true
echo "== rodar OpenCode grátis em repositório temporário =="
(cd "$TMP" && "$BIN" rodar teste-opencode opencode-gratis --prompt "$PROMPT" --pasta-agentes "$TMP/agentes" --tentativas 1) || true

echo "== arquivos criados no temporário =="
git -C "$TMP" status --short
