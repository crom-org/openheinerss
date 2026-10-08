#!/bin/sh
printf 'Claude iniciou a tarefa\n'
printf 'Resultado Claude\n'
if [ "${FAKE_FAIL:-}" = "1" ]; then exit 13; fi
