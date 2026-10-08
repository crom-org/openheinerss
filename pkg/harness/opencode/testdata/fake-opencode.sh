#!/bin/sh
printf 'OpenCode iniciou a tarefa\n'
printf 'Resultado OpenCode\n'
if [ "${FAKE_FAIL:-}" = "1" ]; then exit 11; fi
