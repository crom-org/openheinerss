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

function mapPermissionMode(mode) {
  // Compatibilidade documentada: permissionMode === 'ask' vira manual.
  switch (mode) {
    case 'always_allow': case 'bypassPermissions': return 'bypassPermissions';
    case 'plan': return 'plan';
    case 'acceptEdits': return 'acceptEdits';
    case 'ask': return 'manual'; // nome do protocolo -> nome aceito pelo SDK instalado
    default: return 'manual'; // "manual" ou vazio
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
    process.env.OPENHEINERSS_CLAUDE_SDK_PATH || '',
    path.join(process.env.HOME || '', '.openheinerss/shims/node_modules/@anthropic-ai/claude-agent-sdk')
  ];
  for (const fb of fallbacks) {
    try {
      if (fb && fs.existsSync(fb)) {
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
      const { cwd, env, model, permissionMode, resume, extraArgs, appendSystemPrompt, additionalDirectories, allowedTools, disallowedTools } = params || {};
      const continueLast = !!(params && params.continue);
      
      activeQuery = sdk.query({
        prompt: inputQueue,
        options: {
          cwd: cwd || process.cwd(),
          env: { ...process.env, ...(env || {}) },
          // O protocolo do openheinerss chama o modo de "ask"; o SDK chama de "default".
          permissionMode: mapPermissionMode(permissionMode),
          ...(permissionMode === 'always_allow' || permissionMode === 'bypassPermissions' ? { allowDangerouslySkipPermissions: true } : {}),
          ...(model ? { model } : {}),
          ...(resume ? { resume } : {}),
          ...(!resume && continueLast ? { continue: true } : {}),
          ...(extraArgs ? { extraArgs } : {}),
          ...(appendSystemPrompt ? { systemPrompt: { type: 'preset', preset: 'claude_code', append: appendSystemPrompt } } : {}),
          ...(additionalDirectories ? { additionalDirectories } : {}),
          ...(allowedTools ? { allowedTools } : {}),
          ...(disallowedTools ? { disallowedTools } : {}),
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
            if (m.type !== 'assistant' && m.type !== 'result') send('agent.raw', { line: JSON.stringify(m) });
            if (m.type === 'assistant') {
              const content = (m.message && m.message.content) || [];
              for (const b of content) {
                if (b.type === 'text' && b.text) send('agent.text', { delta: b.text });
                else if (b.type === 'thinking' && b.thinking) send('agent.thinking', { delta: b.thinking });
                else if (b.type === 'tool_use') send('agent.tool_call', { callId: b.id, tool: b.name, input: b.input });
              }
            } else if (m.type === 'result') {
              const u = m.usage || {};
              send('agent.usage', {
                inputTokens: u.input_tokens || 0,
                outputTokens: u.output_tokens || 0,
                costUsd: m.total_cost_usd || 0,
                sessionId: m.session_id || ''
              });
              if (m.is_error) send('agent.error', { message: (typeof m.result === 'string' && m.result) || 'Claude Code encerrou com erro' });
              send('agent.complete', { reason: m.subtype || 'completed', sessionId: m.session_id || '' });
            }
          }
          send('agent.complete', { reason: 'stream_closed' });
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
