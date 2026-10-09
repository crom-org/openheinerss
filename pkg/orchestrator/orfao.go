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

// MotivoOrfao é o motivo gravado no meta.json de um agente cujo processo morreu sem dar FIM.
const MotivoOrfao = "órfão"

// codigoOrfao é o código gravado no fim de um órfão (o processo nunca devolveu um código).
const codigoOrfao = -1

// inicioProcesso devolve o horário de início do processo como o kernel o registra (campo 22 de
// /proc/<pid>/stat, em ticks desde o boot), para não confundir um PID reutilizado com o original.
// Fora do Linux (sem /proc) devolve "".
func inicioProcesso(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')') // o campo 2 (comm) pode ter espaços e parênteses
	if i < 0 {
		return ""
	}
	campos := strings.Fields(s[i+1:]) // começa no campo 3
	if len(campos) < 20 {
		return ""
	}
	return campos[19]
}

// estaVivo diz se o processo registrado no meta ainda é o mesmo: PID vivo e, quando o meta guarda
// inicio_pid, com o mesmo horário de início (PID reutilizado por outro processo conta como morto).
func estaVivo(m meta) bool {
	if !processAlive(m.PID) {
		return false
	}
	if m.InicioPID == "" {
		return true
	}
	atual := inicioProcesso(m.PID)
	return atual == "" || atual == m.InicioPID
}

// metaOrfao: sem fim e com o processo morto (ou trocado).
func metaOrfao(m meta) bool { return m.Fim == "" && !estaVivo(m) }

// fecharOrfao grava fim, código -1 e motivo "órfão" no meta e uma linha FIM no log. Não manda sinal.
func fecharOrfao(logs, nome string, m meta, now time.Time) error {
	code := codigoOrfao
	m.Fim, m.Codigo, m.Motivo = now.Format(time.RFC3339), &code, MotivoOrfao
	if err := writeMeta(filepath.Join(logs, nome+".meta.json"), m); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logs, nome+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil // sem log, o meta fechado já basta
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\nFIM %s código %d (órfão: o processo morreu sem fechar a execução)\n", now.Format("15:04"), code)
	return err
}

// paiMorto diz se o pai de um filho ainda sem fim está órfão (processo morto sem FIM no meta).
// Só informa: nada é alterado no filho.
func paiMorto(logs string, m meta) bool {
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
	return json.Unmarshal(b, &pai) == nil && metaOrfao(pai)
}
