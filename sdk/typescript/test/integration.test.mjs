import test from "node:test";
import assert from "node:assert/strict";
import { Openheinerss } from "../dist/index.js";

const bin = process.env.OPENHEINERSS_BIN || "openheinerss";
test("SDK TypeScript conversa com o servidor real", async () => {
  const client = new Openheinerss({ binPath: bin, transport: "stdio", options: { harness: "mock" } });
  try {
    await client.registerHarness({ name: "ts-test-harness", base: "mock" });
    assert.ok((await client.listHarnesses()).some((h) => h.id === "ts-test-harness"));
    await client.subscribeEvents({ projeto: "teste-ts" }, { "orq.fim": () => {} });
    const limits = await client.getLimits();
    assert.ok(Array.isArray(limits.instancias));
    const run = await client.run({ nome: "teste-ts", motor: "mock", texto: "responda OK", cwd: process.cwd(), projeto: "teste-ts" });
    assert.match(run.id, /^rodar-/);
    await client.stopRun({ id: run.id });
    assert.ok(Array.isArray((await client.listRuns({ projeto: "teste-ts" })).agentes));
  } finally { client.close(); }
});
