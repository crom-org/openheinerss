#!/usr/bin/env node
// Cliente WebSocket mínimo para conferir os eventos orq.* (Node 22+, sem dependências).
//
// Uso: node scripts/ws-cliente.mjs <nome> <motor> [--cwd <repositório>] [--url ws://127.0.0.1:4820/ws]
//                                   [--decidir permitir|negar] [--timeout 120]
// Assina os eventos, lança `rodar.iniciar`, imprime cada mensagem e sai com o código do orq.fim.
const args = process.argv.slice(2);
const pos = [];
const opt = { url: 'ws://127.0.0.1:4820/ws', cwd: process.cwd(), decidir: 'permitir', timeout: '120' };
for (let i = 0; i < args.length; i++) {
  if (args[i].startsWith('--')) opt[args[i].slice(2)] = args[++i];
  else pos.push(args[i]);
}
const [nome, motor] = pos;
if (!nome || !motor) {
  console.error('uso: ws-cliente.mjs <nome> <motor> [--cwd dir] [--url ws://...] [--decidir permitir|negar] [--timeout s]');
  process.exit(64);
}

const ws = new WebSocket(opt.url);
const limite = setTimeout(() => { console.error('tempo esgotado sem orq.fim'); process.exit(124); }, Number(opt.timeout) * 1000);
let id = 0;
const enviar = (method, params) => ws.send(JSON.stringify({ jsonrpc: '2.0', id: ++id, method, params }));

ws.onerror = (e) => { console.error('erro de conexão:', e.message ?? e); process.exit(1); };
ws.onopen = () => {
  enviar('eventos.assinar', { cwd: opt.cwd });
  enviar('rodar.iniciar', { nome, motor, cwd: opt.cwd });
};
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.error) { console.log('RESPOSTA ERRO', JSON.stringify(msg.error)); process.exit(1); }
  if (msg.id !== undefined) { console.log('RESPOSTA', msg.id, JSON.stringify(msg.result)); return; }
  if (!msg.method?.startsWith('orq.')) return;
  console.log(msg.method, JSON.stringify(msg.params));
  if (msg.method === 'orq.precisa_decisao') enviar('rodar.decidir', { run: msg.params.run, id: msg.params.id, geracao: msg.params.geracao, resposta: opt.decidir });
  if (msg.method === 'orq.fim') {
    clearTimeout(limite);
    ws.close();
    process.exit(msg.params.codigo);
  }
};
