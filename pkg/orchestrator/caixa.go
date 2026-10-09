package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// Caixa de entrada de um agente vivo: logs/<nome>.caixa/<id>.json (0600, gravação atômica).
// Quem manda (CLI `agentes mensagem`, RPC rodar.mensagem) só grava o arquivo; o runner do `rodar`
// vigia a pasta e entrega o texto ao motor.

// Estados de uma mensagem.
const (
	MsgPendente    = "pendente"
	MsgEntregue    = "entregue"
	MsgNaoEntregue = "nao_entregue" // o agente terminou antes da entrega
)

// Modos de entrega.
const (
	// ModoVivo: o texto entrou no processo do motor durante o turno (Claude com stream-json).
	ModoVivo = "vivo"
	// ModoRetomada: o texto foi entregue no começo de um novo turno da mesma conversa nativa (--resume).
	ModoRetomada = "retomada"
)

// MensagemMax é o tamanho máximo do texto de uma mensagem, em bytes.
const MensagemMax = 256 * 1024

// Mensagem é uma entrada da caixa de um agente.
type Mensagem struct {
	ID         string `json:"id"`
	Agente     string `json:"agente"`
	Projeto    string `json:"projeto,omitempty"`
	Texto      string `json:"texto"`
	Estado     string `json:"estado"`
	Modo       string `json:"modo,omitempty"`
	Em         string `json:"em"`
	EntregueEm string `json:"entregue_em,omitempty"`
}

// Recibo é o estado no vocabulário da Central: "entregue" ou "pendente".
func (m Mensagem) Recibo() string {
	if m.Estado == MsgEntregue {
		return MsgEntregue
	}
	return MsgPendente
}

func caixaDir(agents, nome string) string { return filepath.Join(agents, "logs", nome+".caixa") }

func novoIDMensagem(now time.Time) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("msg-%s-%s", now.UTC().Format("20060102T150405.000"), hex.EncodeToString(b))
}

func gravarMensagem(dir string, m Mensagem) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".msg-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_ = tmp.Chmod(0600)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, m.ID+".json")); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// EnviarMensagem põe o texto na caixa do agente vivo. Espera até `espera` pela entrega do runner e
// devolve a mensagem no estado visto ao fim da espera (entregue ou pendente).
func EnviarMensagem(agentsDir, nome, projeto, texto string, now time.Time, espera time.Duration) (Mensagem, error) {
	if !nomeValido.MatchString(nome) {
		return Mensagem{}, fmt.Errorf("nome de agente inválido %q", nome)
	}
	if strings.TrimSpace(texto) == "" {
		return Mensagem{}, fmt.Errorf("mensagem vazia")
	}
	if len(texto) > MensagemMax {
		return Mensagem{}, fmt.Errorf("mensagem grande demais (%d bytes; máximo %d)", len(texto), MensagemMax)
	}
	logs := filepath.Join(agentsDir, "logs")
	b, err := os.ReadFile(filepath.Join(logs, nome+".meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Mensagem{}, fmt.Errorf("agente %q não encontrado", nome)
		}
		return Mensagem{}, err
	}
	var m meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Mensagem{}, fmt.Errorf("ler meta de %s: %w", nome, err)
	}
	if m.Fim != "" {
		return Mensagem{}, fmt.Errorf("agente %q já terminou (código %d); não há quem receba a mensagem", nome, valueOr(m.Codigo, 0))
	}
	if !estaVivo(m) {
		return Mensagem{}, fmt.Errorf("agente %q não está rodando (processo %d morto sem FIM)", nome, m.PID)
	}
	dir := caixaDir(agentsDir, nome)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Mensagem{}, err
	}
	msg := Mensagem{ID: novoIDMensagem(now), Agente: nome, Projeto: projeto, Texto: texto, Estado: MsgPendente, Em: now.Format(time.RFC3339)}
	if err := gravarMensagem(dir, msg); err != nil {
		return Mensagem{}, err
	}
	deadline := time.Now().Add(espera)
	for time.Now().Before(deadline) && msg.Estado == MsgPendente {
		time.Sleep(50 * time.Millisecond)
		if cur, ok := lerMensagem(filepath.Join(dir, msg.ID+".json")); ok {
			msg = cur
		}
	}
	return msg, nil
}

func lerMensagem(path string) (Mensagem, bool) {
	var m Mensagem
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &m) != nil || m.ID == "" {
		return m, false
	}
	return m, true
}

// ListarMensagens devolve a caixa do agente em ordem de chegada.
func ListarMensagens(agentsDir, nome string) []Mensagem {
	entries, err := os.ReadDir(caixaDir(agentsDir, nome))
	if err != nil {
		return nil
	}
	var out []Mensagem
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if m, ok := lerMensagem(filepath.Join(caixaDir(agentsDir, nome), e.Name())); ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MensagensPendentes são as mensagens da caixa ainda não entregues.
func MensagensPendentes(agentsDir, nome string) []Mensagem {
	var out []Mensagem
	for _, m := range ListarMensagens(agentsDir, nome) {
		if m.Estado == MsgPendente {
			out = append(out, m)
		}
	}
	return out
}

func marcarMensagem(agentsDir, nome string, m Mensagem, estado, modo string, now time.Time) {
	m.Estado, m.Modo = estado, modo
	if estado == MsgEntregue {
		m.EntregueEm = now.Format(time.RFC3339)
	}
	_ = gravarMensagem(caixaDir(agentsDir, nome), m)
}

// textoMensagens monta o prompt de um turno de retomada com as mensagens recebidas.
func textoMensagens(msgs []Mensagem) string {
	var b strings.Builder
	b.WriteString("--- MENSAGEM DO OPERADOR ---\nO operador enviou, com o agente já em execução, o recado abaixo. Leve-o em conta e continue a tarefa.\n")
	for _, m := range msgs {
		fmt.Fprintf(&b, "\n[%s]\n%s\n", m.ID, strings.TrimSpace(m.Texto))
	}
	return b.String()
}

// textoMensagemViva é o texto de uma mensagem entregue dentro do turno.
func textoMensagemViva(m Mensagem) string {
	return "--- MENSAGEM DO OPERADOR [" + m.ID + "] ---\n" + strings.TrimSpace(m.Texto)
}

// caixaIntervalo é o intervalo com que o runner confere a caixa do agente.
const caixaIntervalo = 250 * time.Millisecond

// caixaRun é o lado do runner: vigia a caixa, registra no log e no evento orq.mensagem e entrega.
type caixaRun struct {
	agents string
	o      Options
	write  func(string)
	vistas map[string]bool
	mu     sync.Mutex
}

func novaCaixaRun(agents string, o Options, write func(string)) *caixaRun {
	return &caixaRun{agents: agents, o: o, write: write, vistas: map[string]bool{}}
}

// pendentes lê a caixa e registra "recebida" na primeira vez que vê cada mensagem.
func (c *caixaRun) pendentes() []Mensagem {
	msgs := MensagensPendentes(c.agents, c.o.Name)
	for _, m := range msgs {
		if !c.vistas[m.ID] {
			c.vistas[m.ID] = true
			c.write(fmt.Sprintf("MENSAGEM recebida %s\n", m.ID))
			c.o.emit(Evento{Tipo: EvMensagem, MensagemID: m.ID, MensagemEstado: "recebida"})
		}
	}
	return msgs
}

func (c *caixaRun) entregue(m Mensagem, modo string) {
	marcarMensagem(c.agents, c.o.Name, m, MsgEntregue, modo, c.o.Now())
	c.write(fmt.Sprintf("MENSAGEM entregue %s (%s)\n", m.ID, modo))
	c.o.emit(Evento{Tipo: EvMensagem, MensagemID: m.ID, MensagemEstado: MsgEntregue, MensagemModo: modo})
}

// vigiar confere a caixa a cada caixaIntervalo até o contexto do motor acabar.
func (c *caixaRun) vigiar(ctx context.Context, h harness.Harness) {
	t := time.NewTicker(caixaIntervalo)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.entregarVivas(h)
		}
	}
}

// entregarVivas passa ao motor, dentro do turno, o que ele aceita ao vivo; o resto fica pendente
// para a retomada no fim do turno.
func (c *caixaRun) entregarVivas(h harness.Harness) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.pendentes()
	vivo, ok := h.(harness.MensageiroVivo)
	if !ok {
		return
	}
	for _, m := range msgs {
		if err := vivo.EnviarVivo(m.ID, textoMensagemViva(m)); err != nil {
			return
		}
		c.entregue(m, ModoVivo)
	}
}

// retomada devolve o texto do próximo turno com as mensagens ainda pendentes (vazio se não houver).
func (c *caixaRun) retomada() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.pendentes()
	if len(msgs) == 0 {
		return ""
	}
	for _, m := range msgs {
		c.entregue(m, ModoRetomada)
	}
	return textoMensagens(msgs)
}
