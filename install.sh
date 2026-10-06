#!/usr/bin/env bash
# Script de instalação do binário Openheinerss
set -euo pipefail

INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "$INSTALL_DIR"

echo "Compilando Openheinerss..."
go build -o openheinerss ./cmd/openheinerss

echo "Copiando binário para $INSTALL_DIR/openheinerss..."
cp openheinerss "$INSTALL_DIR/openheinerss"
chmod +x "$INSTALL_DIR/openheinerss"

echo "✅ Openheinerss instalado com sucesso!"
"$INSTALL_DIR/openheinerss" version
echo "Para verificar o ambiente, execute: openheinerss doctor"
