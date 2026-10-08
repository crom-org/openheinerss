#!/bin/sh
printf 'Aider: analisando o pedido\n'
printf 'Resposta do Aider\n'
if [ "${FAKE_FAIL:-}" = "1" ]; then exit 7; fi
