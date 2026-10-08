#!/bin/sh
# Exemplo mínimo: stdin recebe o prompt; stdout são eventos NDJSON.
IFS= read -r prompt
printf '%s\n' '{"type":"text","text":"prompt recebido"}'
printf '%s\n' '{"type":"usage","inputTokens":1,"outputTokens":1,"totalTokens":2}'
printf '%s\n' '{"type":"end","reason":"completed"}'
