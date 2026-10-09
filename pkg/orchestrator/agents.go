package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Agente é o estado resumido de uma execução encontrada na pasta de logs.
type Agente struct {
	Nome        string   `json:"nome"`
	Projeto     string   `json:"projeto,omitempty"`
	Motor       string   `json:"motor,omitempty"`
	Modelo      string   `json:"modelo,omitempty"`
	Tentativa   int      `json:"tentativa"`
	Estado      string   `json:"estado"`
	Inicio      string   `json:"inicio,omitempty"`
	Duracao     string   `json:"duracao,omitempty"`
	UltimaLinha string   `json:"ultima_linha,omitempty"`
	Codigo      *int     `json:"codigo,omitempty"`
	PID         int      `json:"pid,omitempty"`
	LogFile     string   `json:"log,omitempty"`
	MetaFile    string   `json:"meta,omitempty"`
	Pai         string   `json:"pai,omitempty"`
	Filhos      []string `json:"filhos,omitempty"`
	// Orfaos são os filhos ainda vivos de um pai que já terminou.
	Orfaos []string `json:"orfaos,omitempty"`
	// Orfao marca um filho vivo cujo pai já terminou.
	Orfao bool `json:"orfao,omitempty"`
	// PaiMorto marca um filho cujo pai morreu sem dar FIM (o filho não é alterado).
	PaiMorto bool `json:"pai_morto,omitempty"`
	// Campos do meta.json úteis para quem acompanha o agente (detector de parado, arquivo de estado).
	UltimoEventoEm string       `json:"ultimo_evento_em,omitempty"`
	Head           string       `json:"head,omitempty"`
	Checkpoints    []Checkpoint `json:"checkpoints,omitempty"`
}

// ListAgents lista os agentes da pasta, inclusive execuções terminadas.
func ListAgents(agentsDir string, now time.Time) ([]Agente, error) {
	logs := filepath.Join(agentsDir, "logs")
	entries, err := os.ReadDir(logs)
	if os.IsNotExist(err) {
		return []Agente{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result []Agente
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".meta.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logs, entry.Name()))
		if err != nil {
			continue
		}
		var m meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".meta.json")
		logPath := filepath.Join(logs, name+".log")
		a := Agente{Nome: name, Projeto: m.Projeto, Motor: m.Motor, Modelo: m.Modelo, Tentativa: m.Tentativa, Inicio: m.Inicio, PID: m.PID, LogFile: logPath, MetaFile: filepath.Join(logs, entry.Name())}
		a.UltimaLinha = lastUsefulLine(logPath)
		a.Pai = m.Pai
		a.UltimoEventoEm, a.Head, a.Checkpoints = m.UltimoEventoEm, m.Head, m.Checkpoints
		if metaOrfao(m) {
			// Processo morto sem FIM (ou PID reutilizado): fecha o meta e libera a vaga.
			if fecharOrfao(logs, name, m, now) == nil {
				code := codigoOrfao
				m.Fim, m.Codigo, m.Motivo = now.Format(time.RFC3339), &code, MotivoOrfao
			}
		}
		for _, f := range listarFilhos(logs, name) {
			a.Filhos = append(a.Filhos, f.Nome)
			if m.Fim != "" && lerFilho(f).Vivo {
				a.Orfaos = append(a.Orfaos, f.Nome)
			}
		}
		if m.Fim != "" && m.Motivo == MotivoOrfao {
			a.Estado = MotivoOrfao
			a.Codigo = m.Codigo
			a.Duracao = durationSince(m.Inicio, m.Fim)
		} else if m.Fim != "" && m.Motivo == MotivoParado {
			a.Estado = fmt.Sprintf("parado (código %d, retomável)", valueOr(m.Codigo, CodigoParado))
			a.Codigo = m.Codigo
			a.Duracao = durationSince(m.Inicio, m.Fim)
		} else if m.Fim != "" {
			a.Estado = fmt.Sprintf("terminou código %d", valueOr(m.Codigo, 0))
			a.Codigo = m.Codigo
			a.Duracao = durationSince(m.Inicio, m.Fim)
		} else if estaVivo(m) {
			a.PaiMorto = paiMorto(logs, m)
			a.Estado = estadoVivo(logs, name, logPath, now)
			a.Orfao = paiTerminou(logs, m)
			a.Duracao = durationSince(m.Inicio, now.Format(time.RFC3339))
		} else {
			a.Estado = MotivoOrfao
			a.Duracao = durationSince(m.Inicio, now.Format(time.RFC3339))
		}
		result = append(result, a)
	}
	return result, nil
}

// paiTerminou diz se o pai de um filho já tem FIM no meta.json (na pasta de logs do pai, gravada no
// meta do filho, ou na mesma pasta de logs).
func paiTerminou(logs string, m meta) bool {
	if m.Pai == "" {
		return false
	}
	dir := m.PaiLogs
	if dir == "" {
		dir = logs
	}
	b, err := os.ReadFile(filepath.Join(dir, m.Pai+".meta.json"))
	if err != nil {
		return false
	}
	var pai meta
	return json.Unmarshal(b, &pai) == nil && pai.Fim != ""
}

func valueOr(n *int, fallback int) int {
	if n == nil {
		return fallback
	}
	return *n
}

func durationSince(start, end string) string {
	a, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return ""
	}
	b, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return ""
	}
	if b.Before(a) {
		return "0s"
	}
	return b.Sub(a).Round(time.Second).String()
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return processAlivePlatform(pid)
}

func staleLog(path string, now time.Time) bool {
	info, err := os.Stat(path)
	return err == nil && now.Sub(info.ModTime()) > staleAfter()
}

// LogParado informa se o log está sem atualização pelo prazo configurado.
// OPENHEINERSS_LOG_PARADO_MIN altera o padrão de 15 minutos.
func LogParado(path string, now time.Time) bool { return staleLog(path, now) }

func staleAfter() time.Duration {
	minutes, err := strconv.Atoi(os.Getenv("OPENHEINERSS_LOG_PARADO_MIN"))
	if err == nil && minutes > 0 {
		return time.Duration(minutes) * time.Minute
	}
	return 15 * time.Minute
}

func lastUsefulLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "FIM ") || strings.HasPrefix(line, "### tentativa") {
			continue
		}
		return curto(line, 240)
	}
	return ""
}

// ShowAgentLog retorna o fim legível do log de um agente.
func ShowAgentLog(agentsDir, name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 80
	}
	b, err := os.ReadFile(filepath.Join(agentsDir, "logs", name+".log"))
	if err != nil {
		return "", err
	}
	all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}

// StopAgent encerra apenas o processo registrado para o agente e seu grupo.
func StopAgent(agentsDir, name string, now time.Time) error {
	_, err := PararAgente(agentsDir, name, now)
	return err
}

// PararAgente é o StopAgent que também diz se o agente era órfão (processo já morto, sem FIM): nesse caso
// não manda sinal algum, só fecha o meta com código -1 e motivo "órfão".
func PararAgente(agentsDir, name string, now time.Time) (orfao bool, err error) {
	metaPath := filepath.Join(agentsDir, "logs", name+".meta.json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(metaPath), 0755); err != nil {
				return false, err
			}
			// A fila/inicialização ainda não tem meta. A marca é consumida
			// pelo Run sob a trava de vagas antes de criar a worktree.
			if err := os.WriteFile(filepath.Join(agentsDir, "logs", name+".cancelado"), []byte(now.Format(time.RFC3339)+"\n"), 0600); err != nil {
				return false, err
			}
			return false, nil
		}
		return false, err
	}
	var m meta
	if err := json.Unmarshal(b, &m); err != nil {
		return false, fmt.Errorf("ler meta de %s: %w", name, err)
	}
	if m.Fim != "" {
		return false, fmt.Errorf("agente %s já terminou", name)
	}
	if m.PID <= 0 {
		return false, fmt.Errorf("agente %s não tem PID válido", name)
	}
	if !estaVivo(m) {
		return true, fecharOrfao(filepath.Join(agentsDir, "logs"), name, m, now)
	}
	if m.Servidor {
		return false, fmt.Errorf("agente %s foi lançado por um servidor (PID %d); pare-o com rodar.parar, não pelo PID", name, m.PID)
	}
	if err := stopAgentProcess(m.PID); err != nil {
		return false, fmt.Errorf("parar agente %s (PID %d): %w", name, m.PID, err)
	}
	// Um rodar vivo trata o SIGTERM sozinho (SIGINT pode vir ignorado de um shell com `&`): fecha meta e log (FIM 130). Só escrevemos o fim
	// quando ele não o fez (processo já morto ou que ignorou o sinal).
	for i := 0; i < 30; i++ {
		if b, err := os.ReadFile(metaPath); err == nil {
			var atual meta
			if json.Unmarshal(b, &atual) == nil && atual.Fim != "" {
				return false, nil
			}
		}
		if !processAlive(m.PID) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	code := 130
	m.Fim, m.Codigo = now.Format(time.RFC3339), &code
	if err := writeMeta(metaPath, m); err != nil {
		return false, err
	}
	logPath := filepath.Join(agentsDir, "logs", name+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\nFIM %s código 130\n", now.Format("15:04"))
	return false, err
}
