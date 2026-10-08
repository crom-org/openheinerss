export type HarnessType = "mock" | "claude-code" | "opencode" | string;
export type HarnessMode = "sdk" | "cli" | "api" | "mock";

export interface SessionOptions {
  harness?: HarnessType;
  mode?: HarnessMode;
  cwd?: string;
  provider?: string;
  model?: string;
  permissionMode?: "ask" | "always_allow" | "plan";
  systemPrompt?: string;
}

export interface HarnessRegistration {
  name: string;
  base?: string;
  displayName?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  model?: string;
  prompt?: "stdin" | "argument";
  finishRegex?: string;
  quotaRegex?: string;
  reserva?: string[];
}

export interface RunOptions {
  nome: string; motor: string; modelo?: string; esforco?: string; prompt?: string;
  retomar?: boolean; pasta?: string; branchBase?: string; cargaMax?: number;
  maxAgentes?: number; tentativas?: number; cotaMax?: number; cwd?: string; projeto?: string;
}
export interface RunStarted { id: string; agente: string; projeto: string; }
export interface AgentInfo { id?: string; agente: string; projeto: string; estado: string; motor?: string; modelo?: string; tentativa?: number; inicio?: string; fim?: string; codigo?: number; pid?: number; log?: string; }
export interface OrchestrationDecision { id: string; agente: string; projeto: string; pergunta: string; opcoes: string[]; }
export interface RunList { agentes: AgentInfo[]; decisoes: OrchestrationDecision[]; }
export interface LimitsWindow { nome: string; percentual: number; reiniciaEm?: string; }
export interface Limits { agora: string; instancias: Array<{ nome: string; base: string; janelas: LimitsWindow[]; }>; }
export interface EventFilter { projeto?: string; agente?: string; cwd?: string; pasta?: string; }
export type OrchestrationEventName = "orq.inicio" | "orq.progresso" | "orq.fim" | "orq.erro" | "orq.precisa_decisao";
export type OrchestrationEvent = { method: OrchestrationEventName; params: Record<string, unknown>; };
export type OrchestrationCallback = (params: Record<string, unknown>) => void;

export interface PermissionRequest {
  sessionId: string;
  requestId: string;
  tool: string;
  command?: string;
  risk?: string;
  allow: () => Promise<void>;
  deny: (reason?: string) => Promise<void>;
}

export interface ToolCall {
  sessionId: string;
  callId: string;
  tool: string;
  input: Record<string, unknown>;
}

export interface ToolResult {
  sessionId: string;
  callId: string;
  status: "success" | "error";
  output: string;
}

export interface OpenheinerssEvents {
  thinking: (delta: string) => void;
  text: (delta: string) => void;
  tool_call: (call: ToolCall) => void;
  tool_result: (result: ToolResult) => void;
  permission: (req: PermissionRequest) => void;
  complete: (reason: string) => void;
  error: (err: { message: string; code?: number }) => void;
  "orq.inicio": OrchestrationCallback;
  "orq.progresso": OrchestrationCallback;
  "orq.fim": OrchestrationCallback;
  "orq.erro": OrchestrationCallback;
  "orq.precisa_decisao": OrchestrationCallback;
}
