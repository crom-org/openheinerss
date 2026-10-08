package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Agente é o estado resumido de uma execução encontrada na pasta de logs.
type Agente struct {
	Nome        string `json:"nome"`
	Projeto     string `json:"projeto,omitempty"`
	Motor       string `json:"motor,omitempty"`
	Modelo      string `json:"modelo,omitempty"`
	Tentativa   int    `json:"tentativa"`
	Estado      string `json:"estado"`
	Inicio      string `json:"inicio,omitempty"`
	Duracao     string `json:"duracao,omitempty"`
	UltimaLinha string `json:"ultima_linha,omitempty"`
	Codigo      *int   `json:"codigo,omitempty"`
	PID         int    `json:"pid,omitempty"`
	LogFile     string `json:"log,omitempty"`
	MetaFile    string `json:"meta,omitempty"`
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
		if m.Fim != "" {
			a.Estado = fmt.Sprintf("terminou código %d", valueOr(m.Codigo, 0))
			a.Codigo = m.Codigo
			a.Duracao = durationSince(m.Inicio, m.Fim)
		} else if processAlive(m.PID) {
			if staleLog(logPath, now) {
				a.Estado = "parado"
			} else {
				a.Estado = "rodando"
			}
			a.Duracao = durationSince(m.Inicio, now.Format(time.RFC3339))
		} else {
			a.Estado = "parado"
			a.Duracao = durationSince(m.Inicio, now.Format(time.RFC3339))
		}
		result = append(result, a)
	}
	return result, nil
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
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

func staleLog(path string, now time.Time) bool {
	info, err := os.Stat(path)
	return err == nil && now.Sub(info.ModTime()) > 15*time.Minute
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
	metaPath := filepath.Join(agentsDir, "logs", name+".meta.json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	var m meta
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("ler meta de %s: %w", name, err)
	}
	if m.Fim != "" {
		return fmt.Errorf("agente %s já terminou", name)
	}
	if m.PID <= 0 {
		return fmt.Errorf("agente %s não tem PID válido", name)
	}
	if m.Servidor {
		return fmt.Errorf("agente %s foi lançado por um servidor (PID %d); pare-o com rodar.parar, não pelo PID", name, m.PID)
	}
	// Só sinalize o grupo quando o processo for seu líder. Assim não atingimos
	// o grupo do shell que lançou o CLI; o PID exato é o fallback seguro.
	if pgid, pgErr := syscall.Getpgid(m.PID); pgErr == nil && pgid == m.PID {
		if err := syscall.Kill(-m.PID, syscall.SIGTERM); err != nil {
			if !errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("parar grupo do agente %s (PID %d): %w", name, m.PID, err)
			}
		}
	} else if err := syscall.Kill(m.PID, syscall.SIGTERM); err != nil {
		if !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("parar agente %s (PID %d): %w", name, m.PID, err)
		}
	}
	// Um rodar vivo trata o SIGTERM sozinho (SIGINT pode vir ignorado de um shell com `&`): fecha meta e log (FIM 130). Só escrevemos o fim
	// quando ele não o fez (processo já morto ou que ignorou o sinal).
	for i := 0; i < 30; i++ {
		if b, err := os.ReadFile(metaPath); err == nil {
			var atual meta
			if json.Unmarshal(b, &atual) == nil && atual.Fim != "" {
				return nil
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
		return err
	}
	logPath := filepath.Join(agentsDir, "logs", name+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\nFIM %s código 130\n", now.Format("15:04"))
	return err
}
