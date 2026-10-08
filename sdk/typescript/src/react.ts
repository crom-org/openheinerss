import { useState, useEffect, useRef, useCallback } from "react";
import { Openheinerss, type ClientConfig } from "./index.js";
import type { PermissionRequest, SessionOptions } from "./types.js";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  text: string;
  timestamp: string;
}

export interface UseOpenheinerssOptions extends ClientConfig {
  autoConnect?: boolean;
}

export function useOpenheinerss(options: UseOpenheinerssOptions = {}) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isThinking, setIsThinking] = useState(false);
  const [thinkingText, setThinkingText] = useState("");
  const [permissionRequest, setPermissionRequest] = useState<PermissionRequest | null>(null);
  const [status, setStatus] = useState<"idle" | "running" | "waiting_permission">("idle");
  const clientRef = useRef<Openheinerss | null>(null);

  useEffect(() => {
    const client = new Openheinerss({
      transport: "websocket",
      wsEndpoint: options.wsEndpoint || `ws://127.0.0.1:${options.port || (typeof process !== "undefined" ? process.env.OPENHEINERSS_PORTA || "4820" : "4820")}`,
      options: options.options,
      ...options,
    });
    clientRef.current = client;

    client.on("thinking", (delta) => {
      setIsThinking(true);
      setThinkingText((prev) => prev + delta);
    });

    client.on("text", (delta) => {
      setIsThinking(false);
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last && last.role === "assistant") {
          return [
            ...prev.slice(0, -1),
            { ...last, text: last.text + delta },
          ];
        }
        return [
          ...prev,
          {
            id: Math.random().toString(36).substring(2, 9),
            role: "assistant",
            text: delta,
            timestamp: new Date().toISOString(),
          },
        ];
      });
    });

    client.on("permission", (req) => {
      setStatus("waiting_permission");
      setPermissionRequest(req);
    });

    client.on("complete", () => {
      setIsThinking(false);
      setThinkingText("");
      setStatus("idle");
    });

    client.on("error", (err) => {
      setIsThinking(false);
      setStatus("idle");
      setMessages((prev) => [
        ...prev,
        {
          id: Math.random().toString(36).substring(2, 9),
          role: "system",
          text: `Erro: ${err.message}`,
          timestamp: new Date().toISOString(),
        },
      ]);
    });

    return () => {
      client.close();
    };
  }, [options.wsEndpoint]);

  const prompt = useCallback(async (text: string) => {
    if (!clientRef.current) return;
    setStatus("running");
    setMessages((prev) => [
      ...prev,
      {
        id: Math.random().toString(36).substring(2, 9),
        role: "user",
        text,
        timestamp: new Date().toISOString(),
      },
    ]);
    await clientRef.current.prompt(text);
  }, []);

  const respondPermission = useCallback(async (requestId: string, allow: boolean) => {
    if (!clientRef.current) return;
    await clientRef.current.respondPermission(requestId, allow);
    setPermissionRequest(null);
    setStatus("running");
  }, []);

  const abort = useCallback(async () => {
    if (!clientRef.current) return;
    await clientRef.current.abort();
    setIsThinking(false);
    setStatus("idle");
  }, []);

  return {
    messages,
    isThinking,
    thinkingText,
    permissionRequest,
    status,
    prompt,
    respondPermission,
    abort,
    client: clientRef.current,
  };
}
