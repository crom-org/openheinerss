import test from "node:test";
import assert from "node:assert/strict";
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
      "orq.precisa_decisao": (p) => { decisao = p; },
    });
    const limits = await client.getLimits();
    assert.ok(Array.isArray(limits.instancias));
    const run = await client.run({ nome: "teste-ts", motor: "mock", texto: "responda OK", cwd: process.cwd(), projeto: "teste-ts" });
    assert.match(run.id, /^rodar-/);
    await new Promise((resolve, reject) => { const t = setInterval(() => { if (decisao) { clearInterval(t); resolve(); } }, 10); setTimeout(() => { clearInterval(t); reject(new Error("timeout de decisão")); }, 1000); });
    await client.decideRun(decisao.id, "permitir");
    await Promise.race([terminou, new Promise((_, reject) => setTimeout(() => reject(new Error("timeout de eventos")), 3000))]);
    assert.deepEqual(eventos.slice(0, 3), ["orq.inicio", "orq.progresso", "orq.fim"]);
    assert.ok(Array.isArray((await client.listRuns({ projeto: "teste-ts" })).agentes));
  } finally { client.close(); }
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
