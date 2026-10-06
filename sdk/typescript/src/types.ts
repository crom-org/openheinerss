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
}
