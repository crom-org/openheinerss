#!/bin/sh
if [ "${FAKE_QUOTA:-}" = "1" ]; then
  printf '%s\n' '{"type":"result","subtype":"error_max_turns","is_error":true,"result":"Usage limit reached for this account","session_id":"sess-quota"}'
  exit 0
fi
printf '%s\n' '{"type":"system","subtype":"init","session_id":"sess-fake"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"planejando"},{"type":"text","text":"Claude iniciou a tarefa"},{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"pwd"}}]},"session_id":"sess-fake"}'
printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":"/tmp","is_error":false}]},"session_id":"sess-fake"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"Resultado Claude"}]},"session_id":"sess-fake"}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.01,"usage":{"input_tokens":3,"output_tokens":5},"session_id":"sess-fake"}'
if [ "${FAKE_FAIL:-}" = "1" ]; then exit 13; fi
