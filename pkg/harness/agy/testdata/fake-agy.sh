#!/bin/sh
printf 'AGY iniciou a tarefa\n'
printf 'Resultado AGY\n'
if [ "${FAKE_FAIL:-}" = "1" ]; then exit 9; fi
