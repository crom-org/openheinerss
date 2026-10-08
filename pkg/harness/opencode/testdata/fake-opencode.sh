#!/bin/sh
printf '%s\n' '{"type":"step_start","sessionID":"ses_fake","part":{"type":"step-start"}}'
printf '%s\n' '{"type":"text","sessionID":"ses_fake","part":{"type":"text","text":"Resultado OpenCode"}}'
printf '%s\n' '{"type":"tool_use","sessionID":"ses_fake","part":{"type":"tool","tool":"glob","callID":"call_fake","state":{"status":"completed","input":{"pattern":"*.go"},"output":"arquivo.go"}}}'
printf '%s\n' '{"type":"step_finish","sessionID":"ses_fake","part":{"type":"step-finish","tokens":{"total":12,"input":10,"output":2}}}'
if [ "${FAKE_STDERR:-}" = "1" ]; then printf 'aviso informativo\n' >&2; fi
if [ "${FAKE_FAIL:-}" = "1" ]; then printf 'erro do OpenCode\n' >&2; exit 11; fi
