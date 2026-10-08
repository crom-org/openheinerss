import { EventEmitter } from "node:events";
import { spawn, type ChildProcess } from "node:child_process";
import type { SessionOptions, HarnessRegistration, PermissionRequest, ToolCall, ToolResult, RunOptions, RunStarted, RunList, Limits, EventFilter, OrchestrationEventName, OrchestrationCallback } from "./types.js";

export * from "./types.js";
export * from "./react.js";

export interface ClientConfig {
  transport?: "stdio" | "websocket";
  wsEndpoint?: string;
  binPath?: string;
  options?: SessionOptions;
  port?: number;
}

export class Openheinerss extends EventEmitter {
  private proc?: ChildProcess;
  private ws?: any;
  private sessionId?: string;
  private reqId = 1;
  private pendingCallbacks = new Map<number, { resolve: (res: any) => void; reject: (err: Error) => void }>();
  private transportReady?: Promise<void>;
  /** Geração do processo serve ao qual o cliente está conectado. */
  public generation?: string;
  private config: ClientConfig;

  constructor(config: ClientConfig = {}) {
    super();
    this.config = {
      transport: config.transport || (typeof window !== "undefined" ? "websocket" : "stdio"),
      wsEndpoint: config.wsEndpoint || `ws://127.0.0.1:${config.port || (typeof process !== "undefined" ? process.env.OPENHEINERSS_PORTA || "4820" : "4820")}`,
      binPath: config.binPath || "openheinerss",
      options: config.options || {},
      ...config,
    };
  }

  /** Registra um harness custom no processo do openheinerss. */
  async registerHarness(spec: HarnessRegistration): Promise<void> {
    await this.ensureTransport();
    await this.sendRPC("harness.register", spec as unknown as Record<string, unknown>);
  }

  async listHarnesses(): Promise<any[]> { await this.ensureTransport(); return (await this.sendRPC("harness.listar", {})).harnesses || []; }
  async run(options: RunOptions): Promise<RunStarted> { await this.ensureTransport(); return this.sendRPC("rodar.iniciar", options as unknown as Record<string, unknown>); }
  async listRuns(filter: EventFilter = {}): Promise<RunList> { await this.ensureTransport(); return this.sendRPC("rodar.listar", filter as Record<string, unknown>); }
  async stopRun(idOrAgent: { id?: string; agente?: string }): Promise<void> { await this.ensureTransport(); await this.sendRPC("rodar.parar", idOrAgent); }
  /** `run` (id da execução, vem em `orq.precisa_decisao`) é opcional; se vier, o servidor confere. */
  /** encerrar (opcional): numa negação, termina a execução (orq.fim código 3, motivo "negado"); ausente vale serve --negar-encerra. */
  async decideRun(id: string, resposta: string, mensagem?: string, run?: string, encerrar?: boolean): Promise<void> { await this.ensureTransport(); await this.sendRPC("rodar.decidir", { id, resposta, mensagem, run, encerrar }); }
  async getLimits(): Promise<Limits> { await this.ensureTransport(); return this.sendRPC("limites.obter", {}); }
  async subscribeEvents(filter: EventFilter = {}, callbacks: Partial<Record<OrchestrationEventName, OrchestrationCallback>> = {}): Promise<void> {
    await this.ensureTransport();
    for (const [name, callback] of Object.entries(callbacks)) if (callback) this.on(name, callback as (...args: any[]) => void);
    await this.sendRPC("eventos.assinar", filter as Record<string, unknown>);
  }

  private async ensureTransport(): Promise<void> {
    if (!this.transportReady) {
      this.transportReady = this.config.transport === "stdio" ? Promise.resolve(this.initStdio()) : this.initWebSocket();
    }
    await this.transportReady;
  }

  /**
   * Conecta e inicializa a sessão com o harness desejado
   */
  async start(options?: SessionOptions): Promise<string> {
    const opts = { ...this.config.options, ...options };

    if (this.config.transport === "stdio") {
      await this.ensureTransport();
    } else {
      await this.ensureTransport();
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
    this.transportReady = undefined;
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
    this.proc.stdin?.on("error", () => {
      this.rejectPending(new Error("Servidor openheinerss encerrou a conexão (falha ao escrever no STDIO)"));
    });

    this.proc.on("error", (err: Error) => {
      this.rejectPending(new Error(`Falha ao iniciar binário openheinerss: ${err.message}`));
      this.emit("error", { message: `Falha ao iniciar binário openheinerss: ${err.message}` });
    });
    this.proc.on("close", (code, signal) => {
      this.rejectPending(new Error(`Servidor openheinerss encerrou a conexão (código ${code ?? "desconhecido"}${signal ? `, sinal ${signal}` : ""})`));
    });
  }

  private async initWebSocket(): Promise<void> {
    return new Promise((resolve, reject) => {
      const WebSocketImpl = typeof WebSocket !== "undefined" ? WebSocket : (globalThis as any).WebSocket;
      if (!WebSocketImpl) {
        return reject(new Error("Ambiente WebSocket não encontrado"));
      }
      this.ws = new WebSocketImpl(this.config.wsEndpoint!);

      this.ws.onopen = () => resolve();
      this.ws.onerror = (err: any) => reject(new Error(`Falha ao conectar no WebSocket: ${err}`));
      this.ws.onclose = () => this.rejectPending(new Error("Servidor openheinerss encerrou a conexão WebSocket"));

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
    if (msg.geracao) this.generation = msg.geracao;
    // Resposta a requisição enviada
    if (msg.id !== undefined && this.pendingCallbacks.has(msg.id)) {
      const cb = this.pendingCallbacks.get(msg.id)!;
      this.pendingCallbacks.delete(msg.id);
      if (msg.error) {
        const err = new Error(msg.error.message);
        (err as any).code = msg.error.code;
        (err as any).data = msg.error.data;
        cb.reject(err);
        return;
      }
      cb.resolve(msg.result);
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
        case "orq.inicio": case "orq.progresso": case "orq.fim": case "orq.erro": case "orq.precisa_decisao":
          this.emit(msg.method, p);
          break;
      }
    }
  }

  private rejectPending(error: Error): void {
    for (const [, cb] of this.pendingCallbacks) cb.reject(error);
    this.pendingCallbacks.clear();
  }

  private sendRPC(method: string, params: Record<string, unknown>): Promise<any> {
    return new Promise((resolve, reject) => {
      const id = this.reqId++;
      this.pendingCallbacks.set(id, { resolve, reject });

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
