#!/bin/sh
if [ "$1" = "exec" ] && [ "$2" = "resume" ]; then
  printf '%s\n' '{"type":"thread.started","thread_id":"thread-resumed"}'
else
  printf '%s\n' '{"type":"thread.started","thread_id":"thread-fake"}'
fi
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"OK"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}'
