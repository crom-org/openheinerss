import { EventEmitter } from "node:events";
import { spawn, type ChildProcess } from "node:child_process";
import type { SessionOptions, PermissionRequest, OpenheinerssEvents, ToolCall, ToolResult } from "./types.js";

export * from "./types.js";
export * from "./react.js";

export interface ClientConfig {
  transport?: "stdio" | "websocket";
  wsEndpoint?: string;
  binPath?: string;
  options?: SessionOptions;
}

export class Openheinerss extends EventEmitter {
  private proc?: ChildProcess;
  private ws?: any;
  private sessionId?: string;
  private reqId = 1;
  private pendingCallbacks = new Map<number, (res: any) => void>();
  private config: ClientConfig;

  constructor(config: ClientConfig = {}) {
    super();
    this.config = {
      transport: config.transport || (typeof window !== "undefined" ? "websocket" : "stdio"),
      wsEndpoint: config.wsEndpoint || "ws://127.0.0.1:4799",
      binPath: config.binPath || "openheinerss",
      options: config.options || {},
      ...config,
    };
  }

  /**
   * Conecta e inicializa a sessão com o harness desejado
   */
  async start(options?: SessionOptions): Promise<string> {
    const opts = { ...this.config.options, ...options };

    if (this.config.transport === "stdio") {
      this.initStdio();
    } else {
      await this.initWebSocket();
    }

    const res = await this.sendRPC("session.create", {
      harness: opts.harness || "mock",
      mode: opts.mode,
      cwd: opts.cwd || (typeof process !== "undefined" ? process.cwd() : "/"),
      provider: opts.provider,
      model: opts.model,
      options: {
        permissionMode: opts.permissionMode || "ask",
        systemPrompt: opts.systemPrompt,
      },
    });

    this.sessionId = res.sessionId;
    return this.sessionId!;
  }

  /**
   * Envia uma instrução textual e anexos para o agente
   */
  async prompt(text: string, images: Array<{ mediaType: string; data: string }> = []): Promise<void> {
    if (!this.sessionId) {
      await this.start();
    }

    await this.sendRPC("session.prompt", {
      sessionId: this.sessionId,
      text,
      images,
    });
  }

  /**
   * Responde uma autorização de ferramenta
   */
  async respondPermission(requestId: string, allow: boolean, message?: string): Promise<void> {
    if (!this.sessionId) return;
    await this.sendRPC("session.permission_respond", {
      sessionId: this.sessionId,
      requestId,
      decision: allow ? "allow" : "deny",
      message,
    });
  }

  /**
   * Interrompe o processamento da sessão
   */
  async abort(): Promise<void> {
    if (!this.sessionId) return;
    await this.sendRPC("session.abort", {
      sessionId: this.sessionId,
    });
  }

  /**
   * Fecha os processos ou conexões
   */
  close(): void {
    if (this.proc) {
      this.proc.kill();
      this.proc = undefined;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = undefined;
    }
  }

  private initStdio(): void {
    this.proc = spawn(this.config.binPath!, ["serve", "--stdio"]);

    let buffer = "";
    this.proc.stdout?.on("data", (chunk: Buffer) => {
      buffer += chunk.toString("utf8");
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const msg = JSON.parse(line);
          this.handleIncoming(msg);
        } catch (_) {}
      }
    });

    this.proc.on("error", (err) => {
      this.emit("error", { message: `Falha ao iniciar binário openheinerss: ${err.message}` });
    });
  }

  private async initWebSocket(): Promise<void> {
    return new Promise((resolve, reject) => {
      const WebSocketImpl = typeof WebSocket !== "undefined" ? WebSocket : require("ws");
      this.ws = new WebSocketImpl(this.config.wsEndpoint!);

      this.ws.onopen = () => resolve();
      this.ws.onerror = (err: any) => reject(new Error(`Falha ao conectar no WebSocket: ${err}`));

      this.ws.onmessage = (event: any) => {
        try {
          const data = typeof event.data === "string" ? event.data : event.data.toString();
          const msg = JSON.parse(data);
          this.handleIncoming(msg);
        } catch (_) {}
      };
    });
  }

  private handleIncoming(msg: any): void {
    // Resposta a requisição enviada
    if (msg.id !== undefined && this.pendingCallbacks.has(msg.id)) {
      const cb = this.pendingCallbacks.get(msg.id)!;
      this.pendingCallbacks.delete(msg.id);
      if (msg.error) {
        const err = new Error(msg.error.message);
        (err as any).code = msg.error.code;
        (err as any).data = msg.error.data;
        throw err;
      }
      cb(msg.result);
      return;
    }

    // Notificação de evento
    if (msg.method) {
      const p = msg.params || {};
      switch (msg.method) {
        case "agent.thinking":
          this.emit("thinking", p.delta);
          break;
        case "agent.text":
          this.emit("text", p.delta);
          break;
        case "agent.tool_call":
          this.emit("tool_call", p as ToolCall);
          break;
        case "agent.tool_result":
          this.emit("tool_result", p as ToolResult);
          break;
        case "agent.permission_request": {
          const permReq: PermissionRequest = {
            ...p,
            allow: async () => this.respondPermission(p.requestId, true),
            deny: async (reason?: string) => this.respondPermission(p.requestId, false, reason),
          };
          this.emit("permission", permReq);
          break;
        }
        case "agent.complete":
          this.emit("complete", p.reason);
          break;
        case "agent.error":
          this.emit("error", p);
          break;
      }
    }
  }

  private sendRPC(method: string, params: Record<string, unknown>): Promise<any> {
    return new Promise((resolve, reject) => {
      const id = this.reqId++;
      this.pendingCallbacks.set(id, resolve);

      const payload = JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n";

      if (this.config.transport === "stdio" && this.proc?.stdin) {
        this.proc.stdin.write(payload);
      } else if (this.ws) {
        this.ws.send(payload);
      } else {
        this.pendingCallbacks.delete(id);
        reject(new Error("Nenhum transporte ativo configurado"));
      }
    });
  }
}
