// Package capacidades descreve o que cada harness lê e aceita: arquivos de instrução, skills, MCP,
// retomada de sessão e permissões. Cada célula traz a fonte da confirmação (ajuda do CLI ou texto
// encontrado no binário instalado) ou o estado "nao_confirmado"; nada é deduzido por semelhança.
package capacidades

import (
	"fmt"
	"strings"

	"github.com/crom-org/openheinerss/pkg/comandos"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/identidade"
)

// Estados possíveis de uma célula.
const (
	Sim           = "sim"
	Nao           = "nao"
	NaoConfirmado = "nao_confirmado"
)

// Item é uma célula da matriz: o valor, o que significa e de onde veio a informação.
type Item struct {
	Nome    string `json:"nome"`
	Estado  string `json:"estado"`
	Detalhe string `json:"detalhe,omitempty"`
	Fonte   string `json:"fonte,omitempty"`
}

// Capacidades é a resposta de `capacidades <harness>` e de harness.capacidades.
type Capacidades struct {
	Harness string   `json:"harness"`
	Base    string   `json:"base"`
	Cadeia  []string `json:"cadeia"`
	// ContaDir é a pasta de login da instância; os arquivos globais ficam nela.
	ContaDir string `json:"contaDir,omitempty"`
	// ConferidoCom diz com qual versão do CLI as células foram conferidas.
	ConferidoCom string `json:"conferidoCom"`
	// Instalada é a versão do CLI vista na última `harness atualizar`/`harness versoes`; AjudaMudou
	// avisa que a ajuda do CLI mudou nessa atualização e as células merecem nova conferência.
	Instalada  string `json:"instalada,omitempty"`
	AjudaMudou bool   `json:"ajudaMudou,omitempty"`
	// Instrucoes lista os arquivos de instrução que o CLI lê sozinho.
	Instrucoes []Item `json:"instrucoes"`
	// ImportaArquivo: o arquivo de instrução aceita incluir outro (@arquivo).
	ImportaArquivo Item `json:"importaArquivo"`
	// Skills lista as pastas de skills que o CLI descobre.
	Skills       []Item `json:"skills"`
	AceitaSkills Item   `json:"aceitaSkills"`
	MCP          Item   `json:"mcp"`
	Retomar      Item   `json:"retomar"`
	Permissoes   Item   `json:"permissoes"`
}

type ficha struct {
	versao       string
	instrucoes   []Item
	importa      Item
	skills       []Item
	aceitaSkills Item
	mcp          Item
	retomar      Item
	permissoes   Item
}

const (
	ajuda = "ajuda do CLI"
	bin   = "texto do binário instalado"
)

var fichas = map[string]ficha{
	"claude-code": {
		versao: "claude 2.1.295",
		instrucoes: []Item{
			{"CLAUDE.md", Sim, "projeto e pasta da conta (global)", bin},
			{"AGENTS.md", NaoConfirmado, "o binário só o cita ao importar configuração do Codex; use CLAUDE.md com @AGENTS.md", bin},
			{"GEMINI.md", Nao, "só aparece no importador de configuração do Gemini CLI", bin},
		},
		importa:      Item{"@arquivo", NaoConfirmado, "documentado pelo Claude Code, mas sem prova na ajuda/binário nesta máquina", ""},
		skills:       []Item{{".claude/skills/<nome>/SKILL.md", Sim, "projeto e <conta>/skills", bin}, {".agents/skills", Nao, "só é lido pelo importador de configuração, não como skill ativa", bin}},
		aceitaSkills: Item{"skills", Sim, "--disable-slash-commands desliga todas as skills", ajuda},
		mcp:          Item{"mcp", Sim, "--mcp-config <arquivo>, --strict-mcp-config", ajuda},
		retomar:      Item{"--resume <id>", Sim, "também --continue, --fork-session e --session-id; chave da opção: claude_session_id", ajuda},
		permissoes:   Item{"--permission-mode", Sim, "acceptEdits, auto, bypassPermissions, manual, dontAsk, plan; --dangerously-skip-permissions", ajuda},
	},
	"codex": {
		versao: "codex-cli 0.159.2",
		instrucoes: []Item{
			{"AGENTS.md", Sim, "projeto e CODEX_HOME (global)", bin},
			{"AGENTS.override.md", Sim, "tem prioridade sobre o AGENTS.md da mesma pasta", bin},
			{"CLAUDE.md", Nao, "só aparece no importador de configuração", bin},
			{"GEMINI.md", Nao, "não aparece no binário", bin},
		},
		importa:      Item{"@arquivo", NaoConfirmado, "sem menção na ajuda nem no binário", ""},
		skills:       []Item{{".agents/skills", Sim, "texto \"skills\" e \".agents/skills\" no binário (leitura de skills do projeto)", bin}, {".codex/skills", NaoConfirmado, "o caminho aparece no binário, sem prova de leitura", bin}},
		aceitaSkills: Item{"skills", Sim, "pastas de skills no binário", bin},
		mcp:          Item{"mcp", Sim, "-c mcp_servers.<nome>.* por execução; codex mcp edita a config", ajuda},
		retomar:      Item{"codex exec resume <id>", Sim, "também --last; resume não aceita -s/--profile; chave: codex_session_id", ajuda},
		permissoes:   Item{"--sandbox / --ask-for-approval", Sim, "-s <modo>, -a <política>, --dangerously-bypass-approvals-and-sandbox", ajuda},
	},
	"opencode": {
		versao: "opencode 1.18.33",
		instrucoes: []Item{
			{"AGENTS.md", Sim, "projeto e ~/.config/opencode (global)", bin},
			{"CLAUDE.md", Sim, "também ~/.claude/CLAUDE.md; a opção disableClaudeCodePrompt desliga a leitura", bin},
			{"CONTEXT.md", Sim, "lido junto com os demais arquivos de instrução", bin},
			{"GEMINI.md", Nao, "não aparece no binário", bin},
		},
		importa:      Item{"@arquivo", NaoConfirmado, "o campo \"instructions\" da config inclui arquivos; @ dentro do AGENTS.md não foi comprovado", ""},
		skills:       []Item{{"~/.claude/skills, ~/.agents/skills", Sim, "skills externas carregadas sozinhas", bin}, {".opencode/skill(s)/<nome>/SKILL.md", Sim, "texto de ajuda embutido no binário", bin}},
		aceitaSkills: Item{"skills", Sim, "SKILL.md nas pastas acima", bin},
		mcp:          Item{"mcp", Sim, "opencode mcp gerencia os servidores da config do usuário", ajuda},
		retomar:      Item{"--session <id>", Sim, "também --continue e --fork; chave: opencode_session_id", ajuda},
		permissoes:   Item{"--auto", Sim, "aprova o que não está explicitamente negado; sem a flag, a permissão vem da config/agente (--agent)", ajuda},
	},
	"aider": {
		versao: "aider 0.86.2",
		instrucoes: []Item{
			{"CONVENTIONS.md", Nao, "não é lido sozinho: só entra por --read CONVENTIONS.md", ajuda},
			{"AGENTS.md", Nao, "a ajuda não cita leitura automática", ajuda},
			{"CLAUDE.md", Nao, "a ajuda não cita leitura automática", ajuda},
		},
		importa:      Item{"@arquivo", Nao, "o aider não tem include; use --read <arquivo> (somente leitura)", ajuda},
		skills:       nil,
		aceitaSkills: Item{"skills", Nao, "a ajuda não tem skills", ajuda},
		mcp:          Item{"mcp", Nao, "nenhuma opção de MCP na ajuda", ajuda},
		retomar:      Item{"--restore-chat-history", NaoConfirmado, "restaura o histórico do arquivo de chat; não há id de sessão nativo; chave: restore_chat_history", ajuda},
		permissoes:   Item{"--yes-always", Sim, "responde sim a toda confirmação; não há modo de aprovação intermediário", ajuda},
	},
	"agy": {
		versao: "agy 1.3.2",
		instrucoes: []Item{
			{"GEMINI.md", Sim, "projeto e ~/.gemini (global)", bin},
			{"AGENTS.md", Sim, "projeto e ~/.gemini (global); \"AGENTS.md\" e \"GEMINI.md\" são carregados como regras", bin},
			{"CLAUDE.md", Nao, "não aparece no binário", bin},
		},
		importa:      Item{"@arquivo", NaoConfirmado, "sem menção na ajuda nem no binário", ""},
		skills:       []Item{{".agents/skills/<nome>/SKILL.md", Sim, "{workspace}/.agents/skills e _agents/skills", bin}},
		aceitaSkills: Item{"skills", Sim, "--disable-slash-commands desliga skills no modo print", ajuda},
		mcp:          Item{"mcp", Nao, "sem flag por execução; agy mcp add/remove/list edita a config do usuário", ajuda},
		retomar:      Item{"--conversation <id>", Sim, "também --continue; chave: conversation", ajuda},
		permissoes:   Item{"--mode / --dangerously-skip-permissions", Sim, "--mode accept-edits|plan, --sandbox", ajuda},
	},
}

// Bases lista os harnesses embutidos que têm ficha.
func Bases() []string { return []string{"claude-code", "codex", "opencode", "aider", "agy"} }

// Para devolve as capacidades do harness ou da instância (custom herdando de uma base).
func Para(nome string) (Capacidades, error) {
	cadeia, err := comandos.Cadeia(harness.CanonicalName(nome))
	if err != nil {
		return Capacidades{}, err
	}
	base := cadeia[0]
	c := Capacidades{Harness: nome, Base: base, Cadeia: cadeia}
	f, ok := fichas[base]
	if !ok {
		// Harness custom por comando: o openheinerss não sabe o que o CLI por trás lê.
		nc := func(n string) Item {
			return Item{n, NaoConfirmado, "harness custom por comando: o openheinerss não conhece o CLI por trás", ""}
		}
		c.ConferidoCom = "não aplicável"
		c.ImportaArquivo, c.AceitaSkills, c.MCP, c.Retomar, c.Permissoes = nc("@arquivo"), nc("skills"), nc("mcp"), nc("retomar"), nc("permissoes")
		return c, nil
	}
	c.ConferidoCom = f.versao
	if e, ok := DoCache(base); ok {
		c.Instalada, c.AjudaMudou = e.Versao, e.AjudaMudou
	}
	c.Instrucoes = append([]Item(nil), f.instrucoes...)
	c.ImportaArquivo, c.Skills, c.AceitaSkills = f.importa, append([]Item(nil), f.skills...), f.aceitaSkills
	c.MCP, c.Retomar, c.Permissoes = f.mcp, f.retomar, f.permissoes
	// O catálogo diz como o mcp.json do openheinerss chega ao harness (ou por que não chega).
	for _, item := range harness.ListCatalog() {
		if item.ID == base && item.MCP != "" {
			c.MCP.Detalhe += "; openheinerss: " + item.MCP
		}
	}
	if id, err := identidade.Para(nome, nil); err == nil {
		c.ContaDir = id.ContaDir
	}
	return c, nil
}

// Todas devolve as capacidades das bases embutidas, na ordem de Bases.
func Todas() []Capacidades {
	out := make([]Capacidades, 0, len(fichas))
	for _, b := range Bases() {
		if c, err := Para(b); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// Linhas formata a matriz para o terminal.
func Linhas(c Capacidades) []string {
	linhas := []string{fmt.Sprintf("%s (base %s, conferido com %s)", c.Harness, c.Base, c.ConferidoCom)}
	if c.Instalada != "" && c.Instalada != c.ConferidoCom && !strings.HasSuffix(c.ConferidoCom, " "+c.Instalada) {
		aviso := "  CLI instalado: " + c.Instalada + " (as células foram conferidas com " + c.ConferidoCom + ")"
		if c.AjudaMudou {
			aviso += "; a ajuda do CLI mudou na última atualização: reconfira"
		}
		linhas = append(linhas, aviso)
	}
	if c.ContaDir != "" {
		linhas = append(linhas, "  conta: "+c.ContaDir)
	}
	lista := func(titulo string, itens []Item) {
		if len(itens) == 0 {
			linhas = append(linhas, "  "+titulo+": (nenhum)")
			return
		}
		linhas = append(linhas, "  "+titulo+":")
		for _, it := range itens {
			linhas = append(linhas, "    "+linha(it))
		}
	}
	lista("instruções", c.Instrucoes)
	lista("skills", c.Skills)
	for _, it := range []struct {
		t string
		i Item
	}{{"importa @arquivo", c.ImportaArquivo}, {"aceita skills", c.AceitaSkills}, {"mcp", c.MCP}, {"retomar", c.Retomar}, {"permissões", c.Permissoes}} {
		linhas = append(linhas, "  "+it.t+": "+resumo(it.i))
	}
	return linhas
}

func linha(it Item) string {
	s := fmt.Sprintf("%-38s %-14s", it.Nome, it.Estado)
	if it.Detalhe != "" {
		s += " " + it.Detalhe
	}
	if it.Fonte != "" {
		s += " [" + it.Fonte + "]"
	}
	return strings.TrimRight(s, " ")
}

func resumo(it Item) string {
	s := it.Estado + " — " + it.Nome
	if it.Detalhe != "" {
		s += ": " + it.Detalhe
	}
	if it.Fonte != "" {
		s += " [" + it.Fonte + "]"
	}
	return s
}
