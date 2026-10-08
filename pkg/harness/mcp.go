package harness

import (
	"context"
	"os"
	"path/filepath"

	"github.com/crom-org/openheinerss/pkg/mcp"
)

// OptionMCP escolhe os servidores MCP entregues ao harness nesta execução:
// false/"nenhum" desliga; "a,b" ou ["a","b"] entrega só esses; ausente = todos os de mcp.json.
const OptionMCP = "mcp"

// OptionSemMCP (true) desliga a entrega dos servidores MCP (--sem-mcp).
const OptionSemMCP = "sem_mcp"

// SelecaoMCP lê a escolha de servidores das opções, depois de cfg.Env e por fim do ambiente
// (OPENHEINERSS_MCP).
func SelecaoMCP(cfg SessionConfig) mcp.Selecao {
	if OpcaoBool(cfg.Options, OptionSemMCP) {
		return mcp.Selecao{Desligado: true}
	}
	switch v := cfg.Options[OptionMCP].(type) {
	case bool:
		return mcp.Selecao{Desligado: !v}
	case string:
		return mcp.SelecaoDeTexto(v)
	case []string, []interface{}:
		nomes := OpcaoLista(cfg.Options, OptionMCP)
		if len(nomes) == 0 {
			return mcp.Selecao{Desligado: true}
		}
		return mcp.Selecao{Nomes: nomes}
	}
	if v, ok := cfg.Env[mcp.EnvSelecao]; ok {
		return mcp.SelecaoDeTexto(v)
	}
	if sel, ok := mcp.SelecaoDoEnv(); ok {
		return sel
	}
	return mcp.Selecao{}
}

// ServidoresMCP resolve os servidores (global + projeto, projeto vence) e aplica a seleção.
func ServidoresMCP(cfg SessionConfig) (map[string]mcp.ServerConfig, error) {
	sel := SelecaoMCP(cfg)
	if sel.Desligado {
		return nil, nil
	}
	servs, err := mcp.Efetivos(cfg.CWD)
	if err != nil {
		return nil, err
	}
	return mcp.Selecionar(servs, sel)
}

// ArquivoTemporario grava data (0600) numa pasta temporária própria, apagada quando ctx terminar
// (Stop cancela o contexto da sessão). Serve para entregar configs por execução sem tocar nas
// configs pessoais do usuário.
func ArquivoTemporario(ctx context.Context, nome string, data []byte) (string, error) {
	dir, err := os.MkdirTemp("", "openheinerss-mcp-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, nome)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	go func() {
		<-ctx.Done()
		_ = os.RemoveAll(dir)
	}()
	return path, nil
}
