// Package comandos lista os comandos nativos (/compact, /model…) de cada harness, diz como a ponte
// os repassa no modo sem tela e mescla as anotações do usuário (comandos.yaml).
// A tela de confirmação do primeiro uso é do cliente (crom-central); aqui só guardamos o estado.
package comandos

// Como a ponte repassa o comando ao harness no modo sem tela (ver docs/PONTE.md).
const (
	RepasseLiteral        = "literal"         // vai como está ao harness, que o interpreta
	RepasseTraduzido      = "traduzido"       // a ponte troca por uma opção/flag equivalente
	RepasseSemEquivalente = "sem_equivalente" // erro claro: só existe na tela do harness
)

// Origem do item na lista.
const (
	OrigemEmbutido   = "embutido"   // catálogo deste arquivo
	OrigemDescoberto = "descoberto" // achado em tempo de execução (arquivos de comandos/skills do harness)
	OrigemUsuario    = "usuario"    // só existe no comandos.yaml
)

type item struct {
	nome, descricao, repasse, detalhe string
}

// padraoDesconhecido é como a ponte trata um /x que não está no catálogo, por harness base.
var padraoDesconhecido = map[string]string{
	"claude-code": RepasseLiteral,
	"codex":       RepasseSemEquivalente,
	"opencode":    RepasseTraduzido,
	"aider":       RepasseLiteral,
	"agy":         RepasseLiteral,
	"mock":        RepasseLiteral,
}

const (
	detClaudeLiteral = "vai literal ao claude -p; se o claude não aceitar o comando sem tela, ele mesmo responde"
	detCodexTela     = "só existe no codex interativo; use harnessArgs ou o comando nativo"
	detOpencodeCmd   = "vira opencode run --command <nome> (argumentos viram a mensagem)"
	detOpencodeTela  = "comando da tela (TUI); não existe no opencode run"
	detAiderLiteral  = "vai literal dentro de --message; o aider interpreta"
	detAgyLiteral    = "vai literal em -p=; o modo print expande comandos de barra"
)

// embutido vem da auditoria docs/PONTE.md e do código dos adaptadores (pkg/harness/*).
var embutido = map[string][]item{
	"claude-code": {
		{"/compact", "Compacta a conversa do Claude Code (resume o histórico para liberar contexto)", RepasseLiteral, detClaudeLiteral},
		{"/clear", "Limpa o histórico da conversa", RepasseLiteral, detClaudeLiteral},
		{"/model", "Troca o modelo das próximas chamadas", RepasseTraduzido, "com argumento vira --model nas próximas chamadas; sem argumento vai literal"},
		{"/effort", "Troca o esforço de raciocínio (low…max)", RepasseTraduzido, "com argumento vira --effort nas próximas chamadas"},
		{"/init", "Cria o CLAUDE.md do projeto", RepasseLiteral, detClaudeLiteral},
		{"/review", "Revisa um pull request", RepasseLiteral, detClaudeLiteral},
		{"/security-review", "Revisão de segurança das mudanças pendentes", RepasseLiteral, detClaudeLiteral},
		{"/pr-comments", "Mostra os comentários de um pull request", RepasseLiteral, detClaudeLiteral},
		{"/cost", "Mostra custo e uso da sessão", RepasseLiteral, detClaudeLiteral},
		{"/context", "Mostra o uso da janela de contexto", RepasseLiteral, detClaudeLiteral},
		{"/memory", "Edita os arquivos de memória (CLAUDE.md)", RepasseLiteral, detClaudeLiteral},
		{"/agents", "Gerencia os subagentes", RepasseLiteral, detClaudeLiteral},
		{"/mcp", "Gerencia os servidores MCP", RepasseLiteral, detClaudeLiteral},
		{"/permissions", "Mostra ou altera as permissões de ferramentas", RepasseLiteral, detClaudeLiteral},
		{"/add-dir", "Adiciona uma pasta de trabalho", RepasseLiteral, "vai literal; para a sessão inteira prefira a opção add_dirs (--add-dir)"},
		{"/hooks", "Gerencia os hooks", RepasseLiteral, detClaudeLiteral},
		{"/config", "Abre as configurações", RepasseLiteral, detClaudeLiteral},
		{"/status", "Mostra versão, modelo e conta", RepasseLiteral, detClaudeLiteral},
		{"/doctor", "Verifica a instalação do Claude Code", RepasseLiteral, detClaudeLiteral},
		{"/resume", "Retoma uma conversa anterior", RepasseLiteral, "vai literal; a ponte já retoma pelo id da sessão (--resume)"},
		{"/help", "Mostra a ajuda do Claude Code", RepasseLiteral, detClaudeLiteral},
	},
	"codex": {
		{"/model", "Troca o modelo das próximas chamadas", RepasseTraduzido, "com argumento vira -m nas próximas chamadas"},
		{"/effort", "Troca o esforço de raciocínio", RepasseTraduzido, "vira -c model_reasoning_effort=<valor>"},
		{"/reasoning", "Troca o esforço de raciocínio (igual a /effort)", RepasseTraduzido, "vira -c model_reasoning_effort=<valor>"},
		{"/new", "Começa uma conversa nova", RepasseTraduzido, "esquece o id da thread; a próxima chamada não usa exec resume"},
		{"/clear", "Começa uma conversa nova (igual a /new)", RepasseTraduzido, "esquece o id da thread"},
		{"/compact", "Compacta a conversa do Codex", RepasseSemEquivalente, "codex exec não tem compactação; use /new ou harnessArgs com -c model_auto_compact_token_limit=N"},
		{"/init", "Cria o AGENTS.md do projeto", RepasseSemEquivalente, detCodexTela},
		{"/review", "Revisa as mudanças", RepasseSemEquivalente, detCodexTela},
		{"/diff", "Mostra o diff do git", RepasseSemEquivalente, detCodexTela},
		{"/status", "Mostra a configuração e o uso", RepasseSemEquivalente, detCodexTela},
		{"/approvals", "Muda o modo de aprovação", RepasseSemEquivalente, "use a opção sandbox ou harnessArgs"},
		{"/mcp", "Lista as ferramentas MCP", RepasseSemEquivalente, detCodexTela},
		{"/quit", "Sai do codex", RepasseSemEquivalente, "o codex exec termina sozinho a cada turno"},
	},
	"opencode": {
		{"/model", "Troca o modelo (provedor/modelo)", RepasseTraduzido, "vira -m nas próximas chamadas"},
		{"/models", "Troca o modelo (igual a /model)", RepasseTraduzido, "vira -m nas próximas chamadas"},
		{"/effort", "Troca a variante de esforço", RepasseTraduzido, "vira --variant nas próximas chamadas"},
		{"/variant", "Troca a variante de esforço (igual a /effort)", RepasseTraduzido, "vira --variant nas próximas chamadas"},
		{"/agent", "Troca o agente do opencode", RepasseTraduzido, "vira --agent nas próximas chamadas"},
		{"/new", "Começa uma sessão nova", RepasseTraduzido, "esquece o id da sessão"},
		{"/clear", "Começa uma sessão nova (igual a /new)", RepasseTraduzido, "esquece o id da sessão"},
		{"/share", "Compartilha a sessão", RepasseTraduzido, "liga --share nas próximas chamadas"},
		{"/thinking", "Mostra o raciocínio", RepasseTraduzido, "liga --thinking nas próximas chamadas"},
		{"/init", "Cria o AGENTS.md do projeto", RepasseTraduzido, detOpencodeCmd},
		{"/compact", "Compacta (resume) a sessão do opencode", RepasseTraduzido, detOpencodeCmd},
		{"/undo", "Desfaz a última mensagem", RepasseTraduzido, detOpencodeCmd},
		{"/redo", "Refaz a mensagem desfeita", RepasseTraduzido, detOpencodeCmd},
		{"/help", "Ajuda da tela", RepasseSemEquivalente, detOpencodeTela},
		{"/sessions", "Lista as sessões na tela", RepasseSemEquivalente, detOpencodeTela},
		{"/agents", "Lista os agentes na tela", RepasseSemEquivalente, detOpencodeTela},
		{"/themes", "Troca o tema", RepasseSemEquivalente, detOpencodeTela},
		{"/editor", "Abre o editor externo", RepasseSemEquivalente, detOpencodeTela},
		{"/details", "Mostra detalhes das ferramentas", RepasseSemEquivalente, detOpencodeTela},
		{"/exit", "Sai do opencode", RepasseSemEquivalente, detOpencodeTela},
		{"/quit", "Sai do opencode", RepasseSemEquivalente, detOpencodeTela},
		{"/q", "Sai do opencode", RepasseSemEquivalente, detOpencodeTela},
	},
	"aider": {
		{"/model", "Troca o modelo das próximas chamadas", RepasseTraduzido, "vira --model nas próximas chamadas (o processo sai a cada turno)"},
		{"/reasoning-effort", "Troca o esforço de raciocínio", RepasseTraduzido, "vira --reasoning-effort nas próximas chamadas"},
		{"/effort", "Troca o esforço (igual a /reasoning-effort)", RepasseTraduzido, "vira --reasoning-effort nas próximas chamadas"},
		{"/add", "Adiciona arquivos ao chat", RepasseLiteral, detAiderLiteral},
		{"/drop", "Tira arquivos do chat", RepasseLiteral, detAiderLiteral},
		{"/read-only", "Adiciona arquivos só para leitura", RepasseLiteral, detAiderLiteral},
		{"/ask", "Pergunta sem editar arquivos", RepasseLiteral, detAiderLiteral},
		{"/architect", "Modo arquiteto (planeja e depois edita)", RepasseLiteral, detAiderLiteral},
		{"/code", "Pede uma mudança no código", RepasseLiteral, detAiderLiteral},
		{"/commit", "Faz commit das mudanças fora do chat", RepasseLiteral, detAiderLiteral},
		{"/undo", "Desfaz o último commit do aider", RepasseLiteral, detAiderLiteral},
		{"/diff", "Mostra o diff desde a última mensagem", RepasseLiteral, detAiderLiteral},
		{"/run", "Roda um comando no shell", RepasseLiteral, detAiderLiteral},
		{"/test", "Roda os testes e corrige erros", RepasseLiteral, detAiderLiteral},
		{"/lint", "Roda o lint e corrige", RepasseLiteral, detAiderLiteral},
		{"/clear", "Limpa o histórico do chat", RepasseLiteral, detAiderLiteral},
		{"/reset", "Limpa o histórico e tira todos os arquivos", RepasseLiteral, detAiderLiteral},
		{"/tokens", "Mostra o uso de tokens do contexto", RepasseLiteral, detAiderLiteral},
		{"/map", "Mostra o mapa do repositório", RepasseLiteral, detAiderLiteral},
		{"/web", "Baixa uma página e adiciona ao chat", RepasseLiteral, detAiderLiteral},
		{"/help", "Ajuda do aider", RepasseLiteral, detAiderLiteral},
	},
	"agy": {
		{"/model", "Troca o modelo das próximas chamadas", RepasseTraduzido, "vira --model nas próximas chamadas"},
		{"/effort", "Troca o esforço (low…max)", RepasseTraduzido, "vira --effort nas próximas chamadas"},
		{"/agent", "Troca o agente", RepasseTraduzido, "vira --agent nas próximas chamadas"},
		{"/mode", "Troca o modo (accept-edits ou plan)", RepasseTraduzido, "vira --mode nas próximas chamadas"},
		{"/new", "Começa uma conversa nova", RepasseTraduzido, "esquece --continue e --conversation"},
		{"/clear", "Começa uma conversa nova (igual a /new)", RepasseTraduzido, "esquece --continue e --conversation"},
		{"/exit", "Sai do agy", RepasseSemEquivalente, "o modo print termina sozinho a cada turno; pare a sessão"},
		{"/quit", "Sai do agy", RepasseSemEquivalente, "o modo print termina sozinho a cada turno; pare a sessão"},
	},
}
