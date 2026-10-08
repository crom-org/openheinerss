package claudecode

// NodeWorkerScript é o script headless Node.js que consome o Claude Agent SDK
// e fala o protocolo NDJSON com o Go Core
const NodeWorkerScript = `
const readline = require('readline');
const fs = require('fs');
const path = require('path');

// Fila assíncrona de entrada para o SDK
class InputQueue {
  constructor() {
    this.items = [];
    this.wake = null;
    this.closed = false;
  }
  push(text) {
    this.items.push({ type: 'user', message: { role: 'user', content: text }, parent_tool_use_id: null });
    if (this.wake) { this.wake(); this.wake = null; }
  }
  close() { this.closed = true; if (this.wake) { this.wake(); this.wake = null; } }
  async *[Symbol.asyncIterator]() {
    while (true) {
      while (this.items.length) yield this.items.shift();
      if (this.closed) return;
      await new Promise((r) => (this.wake = r));
    }
  }
}

function send(method, params = {}) {
  process.stdout.write(JSON.stringify({ method, params }) + '\n');
}

let sdk = null;
try {
  sdk = require('@anthropic-ai/claude-agent-sdk');
} catch (e) {
  // Procura em caminhos conhecidos como fallback
  const fallbacks = [
    '/home/j/Documentos/GitHub/claude-code-open/app/node_modules/@anthropic-ai/claude-agent-sdk',
    path.join(process.env.HOME || '', '.openheinerss/shims/node_modules/@anthropic-ai/claude-agent-sdk')
  ];
  for (const fb of fallbacks) {
    try {
      if (fs.existsSync(fb)) {
        sdk = require(fb);
        break;
      }
    } catch (_) {}
  }
}

const pendingPermissions = new Map();
let inputQueue = new InputQueue();
let activeQuery = null;

const rl = readline.createInterface({ input: process.stdin, output: process.stdout, terminal: false });

rl.on('line', async (line) => {
  const trimmed = line.trim();
  if (!trimmed) return;
  try {
    const msg = JSON.parse(trimmed);
    const { method, params } = msg;

    if (method === 'init') {
      if (!sdk) {
        send('agent.error', { message: '@anthropic-ai/claude-agent-sdk não encontrado no ambiente Node.' });
        return;
      }
      const { cwd, env, model, permissionMode } = params || {};
      
      activeQuery = sdk.query({
        prompt: inputQueue,
        options: {
          cwd: cwd || process.cwd(),
          env: { ...process.env, ...(env || {}) },
          // O protocolo do openheinerss chama a opção de "ask", mas o
          // Claude Code atual expõe esse modo como "manual" no CLI interno.
          permissionMode: permissionMode === 'ask' || !permissionMode ? 'manual' : permissionMode,
          canUseTool: (toolName, input, o) => {
            return new Promise((resolve) => {
              const reqId = o.requestId || 'perm_' + Math.random().toString(36).substring(2, 9);
              pendingPermissions.set(reqId, resolve);
              send('agent.permission_request', {
                requestId: reqId,
                tool: toolName,
                command: input && input.command ? input.command : JSON.stringify(input),
                risk: toolName === 'Bash' ? 'high' : 'medium'
              });
            });
          }
        }
      });

      // Consome o stream do SDK
      (async () => {
        try {
          for await (const m of activeQuery) {
            if (m.type === 'stream_event') {
              const evt = m.event;
              if (evt && evt.type === 'content_block_delta') {
                if (evt.delta && evt.delta.type === 'text_delta') {
                  send('agent.text', { delta: evt.delta.text });
                } else if (evt.delta && evt.delta.type === 'thinking_delta') {
                  send('agent.thinking', { delta: evt.delta.thinking });
                }
              }
            } else if (m.type === 'assistant') {
              // Bloco completo
            }
          }
          send('agent.complete', { reason: 'turn_ended' });
        } catch (err) {
          send('agent.error', { message: err.message });
        }
      })();

      send('ready', { status: 'ok' });
    } else if (method === 'prompt') {
      inputQueue.push(params.text || '');
    } else if (method === 'permission_respond') {
      const { requestId, allow } = params;
      const resolver = pendingPermissions.get(requestId);
      if (resolver) {
        pendingPermissions.delete(requestId);
        resolver({ behavior: allow ? 'allow' : 'deny' });
      }
    } else if (method === 'abort') {
      inputQueue.close();
      process.exit(0);
    }
  } catch (err) {
    send('agent.error', { message: 'Erro no worker: ' + err.message });
  }
});
`
