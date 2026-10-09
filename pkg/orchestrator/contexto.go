package orchestrator

import (
	"fmt"
	"os"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// EvContexto: o contexto da sessão passou do limite (Mensagem traz a linha do aviso).
const EvContexto = "contexto"

// MaxReiniciosContexto é o máximo de reinícios em sessão nova por execução; depois disso só avisa.
const MaxReiniciosContexto = 3

// BaseDe devolve o harness base de uma instância (ou o próprio nome).
func BaseDe(nome string) string { return baseHarness(nome) }

// RaizDoRepo devolve a raiz do repositório de cwd (a mesma que o rodar usa para ler a configuração).
func RaizDoRepo(cwd string) (string, error) { return gitRoot(cwd) }

// ctxRun acompanha o tamanho do contexto de uma execução do rodar.
type ctxRun struct {
	cfg       config.Contexto
	reinicios int
	avisou    bool  // já avisou nesta sessão
	tokens    int64 // último tamanho de contexto visto
	pendente  bool  // a próxima partida é uma sessão nova depois de passar do limite
}

// novoCtxRun resolve a regra efetiva (configuração + flags do rodar). Erro de arquivo inválido sobe daqui.
func novoCtxRun(repo string, o Options) (*ctxRun, error) {
	c, err := config.ContextoEfetivo(repo, o.Motor, baseHarness(o.Motor))
	if err != nil {
		return nil, err
	}
	c, err = config.AplicarFlagsContexto(c, o.LimiteContexto, o.AcaoContexto)
	if err != nil {
		return nil, err
	}
	return &ctxRun{cfg: c}, nil
}

// tamanhoContexto estima o contexto pelo input do turno; só o total quando o input não veio.
func tamanhoContexto(u protocol.UsageParams) int64 {
	if u.InputTokens > 0 {
		return u.InputTokens
	}
	return u.TotalTokens
}

// observar lê um evento e diz se o contexto está no limite ou acima (só vale para eventos de uso).
func (c *ctxRun) observar(ev harness.Event) bool {
	if !c.cfg.Ligado() || ev.Type != harness.EventUsage {
		return false
	}
	u, ok := ev.Payload.(protocol.UsageParams)
	if !ok {
		return false
	}
	if n := tamanhoContexto(u); n > 0 {
		c.tokens = n
	}
	return c.tokens >= int64(c.cfg.LimiteTokens)
}

// aoPassar avisa (uma vez por sessão) e diz se o runner deve encerrar o turno e recomeçar em sessão nova.
func (c *ctxRun) aoPassar(o Options, projeto, motor string, write func(string)) (reiniciar bool) {
	reiniciar = c.cfg.Acao == config.AcaoNovaSessao && c.reinicios < MaxReiniciosContexto
	if !c.avisou {
		c.avisou = true
		linha := fmt.Sprintf("[contexto] %d tokens >= limite %d (origem: %s)", c.tokens, c.cfg.LimiteTokens, c.cfg.OrigemLimite)
		write("\n" + linha + "\n")
		if c.cfg.Acao == config.AcaoNovaSessao {
			if reiniciar {
				write(fmt.Sprintf("[contexto] encerrando o turno e recomeçando em sessão nova (reinício %d de %d)\n", c.reinicios+1, MaxReiniciosContexto))
			} else {
				write(fmt.Sprintf("[contexto] máximo de %d reinícios atingido; só avisando\n", MaxReiniciosContexto))
			}
		}
		o.emit(Evento{Tipo: EvContexto, Motor: motor, Mensagem: linha})
		if err := appendEventLogContexto(o, projeto, motor, c.tokens, c.cfg); err != nil {
			write("AVISO: não gravei o contexto no log de eventos: " + err.Error() + "\n")
		}
	}
	if reiniciar {
		c.reinicios++
		c.pendente = true
	}
	return reiniciar
}

// textoContinuacao monta o que o motor recebe, além do prompt original, ao recomeçar em sessão nova
// depois de passar do limite de contexto. É o ponto único para trocar a continuação (ex.: por um arquivo de estado).
func (c *ctxRun) textoContinuacao(estado string) string {
	if estado == "" {
		estado = continuation
	}
	t := estado + fmt.Sprintf("\n\nObs.: a sessão anterior foi encerrada por passar do limite de contexto (%d tokens >= %d); esta é uma sessão nova, sem o histórico dela.", c.tokens, c.cfg.LimiteTokens)
	c.pendente, c.avisou, c.tokens = false, false, 0 // a sessão nova começa limpa
	return t
}

// appendEventLogContexto grava a linha orq.contexto no log de eventos (--eventos-log / eventos_log), se houver.
func appendEventLogContexto(o Options, projeto, motor string, tokens int64, cfg config.Contexto) error {
	path := o.EventLog
	if path == "" {
		if spec, ok := harness.CustomSpecFor(motor); ok {
			path = spec.EventLog
		}
	}
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	linha := fmt.Sprintf("[%s] orq.contexto %s tokens=%d limite=%d acao=%s origem=%s\n", projeto, o.Name, tokens, cfg.LimiteTokens, cfg.Acao, cfg.OrigemLimite)
	_, err = f.WriteString(linha)
	return err
}
