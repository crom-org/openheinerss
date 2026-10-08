package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	WorkspaceDirName   = ".openheinerss"
	SessionsDirName    = "sessions"
	CheckpointsDirName = "checkpoints"
	ConfigFileName     = "config.yaml"
	McpFileName        = "mcp.json"
)

// ProjectConfig configurações do projeto local
type ProjectConfig struct {
	Version        string `json:"version" yaml:"version"`
	DefaultHarness string `json:"default_harness" yaml:"default_harness"`
	DefaultMode    string `json:"default_mode" yaml:"default_mode"`
	PermissionMode string `json:"permission_mode" yaml:"permission_mode"`
	EventosLog     string `json:"eventos_log" yaml:"eventos_log"`
	// RegrasPadrao permite substituir o arquivo de regras padrão do runner.
	// SemRegrasPadrao desliga as regras curtas embutidas e o arquivo configurado.
	RegrasPadrao    string `json:"regras_padroes" yaml:"regras_padroes"`
	SemRegrasPadrao bool   `json:"sem_regras_padroes" yaml:"sem_regras_padroes"`
}

// LoadProject lê a configuração opcional do projeto. Arquivo ausente não é erro.
func LoadProject(cwd string) (ProjectConfig, error) {
	var cfg ProjectConfig
	b, err := os.ReadFile(filepath.Join(cwd, WorkspaceDirName, ConfigFileName))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("ler %s: %w", ConfigFileName, err)
	}
	return cfg, nil
}

// InitWorkspace cria a estrutura de diretórios .openheinerss na raiz do projeto
func InitWorkspace(cwd string) error {
	baseDir := filepath.Join(cwd, WorkspaceDirName)

	dirs := []string{
		baseDir,
		filepath.Join(baseDir, SessionsDirName),
		filepath.Join(baseDir, CheckpointsDirName),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("falha ao criar pasta %s: %w", d, err)
		}
	}

	// Cria config.yaml padrão se não existir
	configFile := filepath.Join(baseDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		defaultConfig := `# Openheinerss Project Configuration
version: "1.0"
default_harness: "mock"
default_mode: "sdk"
permission_mode: "ask"
`
		if err := os.WriteFile(configFile, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("falha ao criar %s: %w", configFile, err)
		}
	}

	// Cria mcp.json padrão se não existir
	mcpFile := filepath.Join(baseDir, McpFileName)
	if _, err := os.Stat(mcpFile); os.IsNotExist(err) {
		defaultMcp := `{
  "$schema": "https://json.schemastore.org/mcp-config",
  "mcpServers": {}
}
`
		if err := os.WriteFile(mcpFile, []byte(defaultMcp), 0644); err != nil {
			return fmt.Errorf("falha ao criar %s: %w", mcpFile, err)
		}
	}

	return nil
}

// GetGlobalDir retorna o diretório de dados do usuário (~/.openheinerss)
func GetGlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, WorkspaceDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	profilesDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// EnvConfigDir aponta a pasta de configuração (instâncias de harness e motores.yaml)
// sem depender da pasta atual. A flag global --config/--configuracao vence a variável.
const EnvConfigDir = "OPENHEINERSS_CONFIG"

var dirOverride string

// SetConfigDir registra a pasta vinda da flag --config (vazio desfaz).
func SetConfigDir(dir string) { dirOverride = dir }

// ConfigDir resolve a pasta de configuração explícita: flag > OPENHEINERSS_CONFIG.
// Retorna "" quando nenhuma das duas foi dada (vale a busca a partir da pasta atual).
// Se a pasta contém .openheinerss/, usa essa subpasta; senão usa a própria pasta
// (que então deve conter harnesses/ e/ou motores.yaml). Pasta inexistente é erro.
func ConfigDir() (string, error) {
	dir, origem := dirOverride, "--config"
	if dir == "" {
		dir, origem = os.Getenv(EnvConfigDir), EnvConfigDir
	}
	if dir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("pasta de configuração %s (%s) não existe ou não é pasta", abs, origem)
	}
	if st, err := os.Stat(filepath.Join(abs, WorkspaceDirName)); err == nil && st.IsDir() {
		return filepath.Join(abs, WorkspaceDirName), nil
	}
	return abs, nil
}
