import test from "node:test";
import assert from "node:assert/strict";
import { chmod, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Openheinerss } from "../dist/index.js";

const bin = process.env.OPENHEINERSS_BIN || "openheinerss";
test("SDK TypeScript conversa com o servidor real", async () => {
  const client = new Openheinerss({ binPath: bin, transport: "stdio", options: { harness: "mock" } });
  try {
    await client.registerHarness({ name: "ts-test-harness", base: "mock" });
    assert.ok((await client.listHarnesses()).some((h) => h.id === "ts-test-harness"));
    const eventos = [];
    let decisao;
    let fim;
    const terminou = new Promise((resolve) => { fim = resolve; });
    await client.subscribeEvents({ projeto: "teste-ts" }, {
      "orq.inicio": () => eventos.push("orq.inicio"),
      "orq.progresso": () => eventos.push("orq.progresso"),
      "orq.fim": () => { eventos.push("orq.fim"); fim(); },
      "orq.precisa_decisao": (p) => {
        decisao = p;
        void client.decideRun(p.id, "permitir", undefined, p.run);
      },
    });
    const limits = await client.getLimits();
    assert.ok(Array.isArray(limits.instancias));
    const run = await client.run({ nome: "teste-ts", motor: "mock", texto: "responda OK", cwd: await repoTemporario(), projeto: "teste-ts" });
    assert.match(run.id, /^rodar-/);
    await new Promise((resolve, reject) => { const t = setInterval(() => { if (decisao) { clearInterval(t); resolve(); } }, 10); setTimeout(() => { clearInterval(t); reject(new Error("timeout de decisão")); }, 3000); });
    await Promise.race([terminou, new Promise((_, reject) => setTimeout(() => reject(new Error("timeout de eventos")), 3000))]);
    assert.deepEqual(eventos.slice(0, 3), ["orq.inicio", "orq.progresso", "orq.fim"]);
    assert.ok(Array.isArray((await client.listRuns({ projeto: "teste-ts" })).agentes));
  } finally { client.close(); }
});

test("SDK TypeScript nega com encerrar e recebe orq.fim código 3", async () => {
  const client = new Openheinerss({ binPath: bin, transport: "stdio" });
  try {
    let fim;
    const terminou = new Promise((resolve) => { fim = resolve; });
    const inicios = [];
    await client.subscribeEvents({ projeto: "teste-ts-negar" }, {
      "orq.inicio": (p) => inicios.push(p),
      "orq.fim": (p) => fim(p),
      "orq.precisa_decisao": (p) => { void client.decideRun(p.id, "negar", undefined, p.run, true); },
    });
    await client.run({ nome: "teste-ts-negar", motor: "mock", texto: "responda OK", cwd: await repoTemporario(), projeto: "teste-ts-negar", tentativas: 2 });
    const p = await Promise.race([terminou, new Promise((_, reject) => setTimeout(() => reject(new Error("timeout de orq.fim")), 5000))]);
    assert.equal(p.codigo, 3);
    assert.equal(p.motivo, "negado");
    assert.equal(inicios.length, 1);
  } finally { client.close(); }
});

test("SDK TypeScript falha pendências quando o servidor chega ao EOF", async () => {
  const pasta = await mkdtemp(join(tmpdir(), "openheinerss caminho com espaco-"));
  const falso = join(pasta, "servidor falso.mjs");
  await writeFile(falso, "process.stdin.once('data', () => setTimeout(() => {}, 30000));\n");
  await chmod(falso, 0o755);
  const client = new Openheinerss({ binPath: falso, transport: "stdio" });
  const pendente = client.getLimits();
  const rejeicao = assert.rejects(() => pendente, /encerrou a conexão/);
  await new Promise((resolve) => setTimeout(resolve, 50));
  client.proc.kill();
  await rejeicao;
  client.close();
});

test("SDK TypeScript entrega thinking como thinking", async () => {
  const client = new Openheinerss({ binPath: bin, transport: "stdio", options: { harness: "mock" } });
  let thinking = 0;
  let terminou;
  const completo = new Promise((resolve) => { terminou = resolve; });
  client.on("thinking", () => thinking++);
  client.on("permission", (pedido) => pedido.allow());
  client.on("complete", () => terminou());
  try {
    await client.prompt("responda OK");
    await Promise.race([completo, new Promise((_, reject) => setTimeout(() => reject(new Error("timeout de sessão")), 3000))]);
    assert.ok(thinking > 0);
  } finally { client.close(); }
});

test("SDK TypeScript manda harnessArgs no JSON-RPC e entrega agent.raw", async () => {
  const pasta = await mkdtemp(join(tmpdir(), "openheinerss-ponte-"));
  const falso = join(pasta, "falso.mjs");
  const registro = join(pasta, "reqs.jsonl");
  await writeFile(falso, `#!/usr/bin/env node
import { appendFileSync } from "node:fs";
import { createInterface } from "node:readline";
const out = (o) => process.stdout.write(JSON.stringify({ jsonrpc: "2.0", ...o }) + "\\n");
createInterface({ input: process.stdin }).on("line", (l) => {
  const req = JSON.parse(l);
  appendFileSync(${JSON.stringify(registro)}, l + "\\n");
  if (req.method === "session.create") out({ id: req.id, result: { sessionId: "s1" } });
  if (req.method === "session.prompt") {
    out({ id: req.id, result: { accepted: true } });
    out({ method: "agent.raw", params: { sessionId: "s1", harness: "falso", stream: "stdout", line: "linha crua" } });
  }
});
`);
  await chmod(falso, 0o755);
  const client = new Openheinerss({ binPath: falso, transport: "stdio" });
  const raw = new Promise((resolve) => client.on("raw", resolve));
  try {
    await client.start({ harness: "falso", effort: "high", harnessArgs: ["--x=a,b", "/compact", "c d"] });
    await client.prompt("/model x");
    const r = await Promise.race([raw, new Promise((_, reject) => setTimeout(() => reject(new Error("sem agent.raw")), 3000))]);
    assert.deepEqual(r, { sessionId: "s1", harness: "falso", stream: "stdout", line: "linha crua" });
  } finally { client.close(); }
  const { readFile } = await import("node:fs/promises");
  const reqs = (await readFile(registro, "utf8")).trim().split("\n").map((l) => JSON.parse(l));
  assert.deepEqual(reqs.find((q) => q.method === "session.create").params.options.harnessArgs, ["--x=a,b", "/compact", "c d"]);
  assert.equal(reqs.find((q) => q.method === "session.create").params.options.effort, "high");
  assert.equal(reqs.find((q) => q.method === "session.prompt").params.text, "/model x");
});

// Repositório git descartável: o teste nunca cria worktrees no repositório real.
async function repoTemporario() {
  const { execFileSync } = await import("node:child_process");
  const pasta = await mkdtemp(join(tmpdir(), "openheinerss-sdk-ts-"));
  execFileSync("git", ["init", "-q", "-b", "main", pasta]);
  execFileSync("git", ["-C", pasta, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"]);
  return pasta;
}

test("SDK TypeScript lista, anota e confirma comandos do harness", async () => {
  const cfg = await mkdtemp(join(tmpdir(), "openheinerss-cmd-ts-"));
  const antes = process.env.OPENHEINERSS_CONFIG;
  process.env.OPENHEINERSS_CONFIG = cfg;
  const client = new Openheinerss({ binPath: bin, transport: "stdio" });
  try {
    const nota = await client.annotateCommand("claude-code", "/compact", "compacta o claude code; o central não usa");
    assert.equal(nota.anotacao, "compacta o claude code; o central não usa");
    assert.equal((await client.confirmCommand("claude-code", "/compact")).confirmado, true);
    const lista = await client.listCommands("codex");
    assert.equal(lista.desconhecido, "sem_equivalente");
    assert.equal(lista.comandos.find((c) => c.nome === "/compact").repasse, "sem_equivalente");
    const claude = await client.listCommands("claude-code");
    assert.equal(claude.arquivo, join(cfg, "comandos.yaml"));
    assert.equal(claude.comandos.find((c) => c.nome === "/compact").confirmado, true);
  } finally {
    client.close();
    if (antes === undefined) delete process.env.OPENHEINERSS_CONFIG; else process.env.OPENHEINERSS_CONFIG = antes;
  }
});

test("SDK TypeScript manda semMcp, mcp e classificarRisco no JSON-RPC", async () => {
  const pasta = await mkdtemp(join(tmpdir(), "openheinerss-mcp-"));
  const falso = join(pasta, "falso.mjs");
  const registro = join(pasta, "reqs.jsonl");
  await writeFile(falso, `#!/usr/bin/env node
import { appendFileSync } from "node:fs";
import { createInterface } from "node:readline";
createInterface({ input: process.stdin }).on("line", (l) => {
  const req = JSON.parse(l);
  appendFileSync(${JSON.stringify(registro)}, l + "\\n");
  if (req.method === "session.create") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: req.id, result: { sessionId: "s1" } }) + "\\n");
});
`);
  await chmod(falso, 0o755);
  for (const opts of [{ mcp: ["fs"], classificarRisco: true }, { semMcp: true }]) {
    const client = new Openheinerss({ binPath: falso, transport: "stdio" });
    try { await client.start({ harness: "falso", ...opts }); } finally { client.close(); }
  }
  const { readFile } = await import("node:fs/promises");
  const criar = (await readFile(registro, "utf8")).trim().split("\n").map((l) => JSON.parse(l)).filter((q) => q.method === "session.create");
  assert.deepEqual(criar[0].params.options.mcp, ["fs"]);
  assert.equal(criar[0].params.options.classificarRisco, true);
  assert.equal(criar[1].params.options.semMcp, true);
});
