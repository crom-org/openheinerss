package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/crom-org/openheinerss/pkg/config"
)

// ServerConfig especifica a inicialização de um servidor MCP
type ServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"` // Para servidores MCP remotos via SSE
}

// Config estrutura o arquivo .openheinerss/mcp.json
type Config struct {
	Schema     string                  `json:"$schema,omitempty"`
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerSummary resumo para exibição de servidores configurados
type ServerSummary struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "stdio" ou "sse"
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Hub coordena a leitura e configuração centralizada de servidores MCP
type Hub struct {
	mu sync.RWMutex
}

var defaultHub = &Hub{}

// GetHub retorna a instância singleton do gerenciador MCP
func GetHub() *Hub {
	return defaultHub
}

// LoadConfig carrega os servidores configurados no projeto
func (h *Hub) LoadConfig(cwd string) (*Config, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	filePath := filepath.Join(cwd, config.WorkspaceDirName, config.McpFileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{MCPServers: make(map[string]ServerConfig)}, nil
		}
		return nil, fmt.Errorf("falha ao ler %s: %w", filePath, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("formato inválido em %s: %w", filePath, err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]ServerConfig)
	}

	return &cfg, nil
}

// ListServers retorna os servidores ativos resumidos
func (h *Hub) ListServers(cwd string) ([]ServerSummary, error) {
	cfg, err := h.LoadConfig(cwd)
	if err != nil {
		return nil, err
	}

	var list []ServerSummary
	for name, s := range cfg.MCPServers {
		t := "stdio"
		if s.URL != "" {
			t = "sse"
		}
		list = append(list, ServerSummary{
			Name:    name,
			Type:    t,
			Command: s.Command,
			URL:     s.URL,
		})
	}
	return list, nil
}

// RegisterServer adiciona ou atualiza um servidor MCP em .openheinerss/mcp.json
func (h *Hub) RegisterServer(cwd, name string, s ServerConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	filePath := filepath.Join(cwd, config.WorkspaceDirName, config.McpFileName)
	cfg, _ := h.LoadConfig(cwd)
	cfg.MCPServers[name] = s

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}
