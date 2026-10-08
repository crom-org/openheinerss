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
  /** Esforço de raciocínio repassado ao harness. */
  effort?: string;
  /** Argumentos nativos extras do harness: vão intactos e na ordem, sem filtro. */
  harnessArgs?: string[];
  /** Não entrega ao harness os servidores de mcp.json (padrão: entrega todos). */
  semMcp?: boolean;
  /** Entrega só estes servidores de mcp.json. */
  mcp?: string[];
  /** Liga (true) ou desliga (false) o classificador de risco opcional; ausente segue o servidor (desligado). */
  classificarRisco?: boolean;
}

/** Nível do classificador de risco opcional (só informa, nunca bloqueia). */
export type Risco = "baixo" | "medio" | "alto";

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
  nome: string; motor: string; modelo?: string; esforco?: string; prompt?: string; texto?: string;
  retomar?: boolean; pasta?: string; branchBase?: string; cargaMax?: number;
  maxAgentes?: number; tentativas?: number; cotaMax?: number; filhosObrigatorios?: boolean; cwd?: string; projeto?: string;
  /** Argumentos nativos extras do harness: vão intactos e na ordem, sem filtro. */
  harnessArgs?: string[];
}
export interface RunStarted { geracao: string; id: string; agente: string; projeto: string; }
export interface AgentInfo { id?: string; agente: string; projeto: string; estado: string; motor?: string; modelo?: string; tentativa?: number; inicio?: string; fim?: string; codigo?: number; pid?: number; log?: string; }
export interface OrchestrationDecision { geracao?: string; run?: string; id: string; agente: string; projeto: string; pergunta: string; opcoes: string[]; }
export interface RunList { agentes: AgentInfo[]; decisoes: OrchestrationDecision[]; }
export interface LimitsWindow { nome: string; percentual: number; reiniciaEm?: string; }
export interface Limits { agora: string; instancias: Array<{ nome: string; base: string; janelas: LimitsWindow[]; }>; }
export interface EventFilter { projeto?: string; agente?: string; cwd?: string; pasta?: string; }
/** Como a ponte repassa o comando no modo sem tela. */
export type CommandRelay = "literal" | "traduzido" | "sem_equivalente";
/** Comando nativo do harness (harness.comandos), já mesclado com a anotação do usuário. */
export interface HarnessCommand { nome: string; descricao: string; repasse: CommandRelay; detalhe?: string; origem: "embutido" | "descoberto" | "usuario"; anotacao?: string; confirmado: boolean; }
export interface HarnessCommandList { harness: string; base: string; cadeia: string[]; desconhecido: CommandRelay; arquivo: string; comandos: HarnessCommand[]; }
export type OrchestrationEventName = "orq.inicio" | "orq.progresso" | "orq.fim" | "orq.erro" | "orq.precisa_decisao" | "orq.filhos_orfaos";
export type OrchestrationEvent = { method: OrchestrationEventName; params: Record<string, unknown> & { geracao: string }; };
export type OrchestrationCallback = (params: Record<string, unknown>) => void;

export interface PermissionRequest {
  sessionId: string;
  requestId: string;
  tool: string;
  command?: string;
  risk?: string;
  /** Só com classificarRisco ligado. */
  risco?: Risco;
  motivoRisco?: string;
  allow: () => Promise<void>;
  deny: (reason?: string) => Promise<void>;
}

export interface ToolCall {
  sessionId: string;
  callId: string;
  tool: string;
  input: Record<string, unknown>;
  /** Só com classificarRisco ligado. */
  risco?: Risco;
  motivoRisco?: string;
}

export interface ToolResult {
  sessionId: string;
  callId: string;
  status: "success" | "error";
  output: string;
}

/** Linha original do harness (`agent.raw`): stdout sem mapeamento ou stderr, como foi escrita. */
export interface RawLine {
  sessionId: string;
  harness: string;
  stream: "stdout" | "stderr" | string;
  line: string;
}

export interface OpenheinerssEvents {
  raw: (raw: RawLine) => void;
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
  "orq.filhos_orfaos": OrchestrationCallback;
}
