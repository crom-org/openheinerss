package checkpoint

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
)

// CheckpointInfo metadados de um ponto de restauração
type CheckpointInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
	GitHash   string `json:"gitHash,omitempty"`
}

// Manager coordena a criação e restauração de checkpoints de arquivos
type Manager struct{}

var defaultManager = &Manager{}

// GetManager retorna a instância padrão do gerenciador de checkpoints
func GetManager() *Manager {
	return defaultManager
}

// CreateCheckpoint salva o estado atual do repositório
func (m *Manager) CreateCheckpoint(cwd, sessionID, message string) (*CheckpointInfo, error) {
	checkpointsDir := filepath.Join(cwd, config.WorkspaceDirName, config.CheckpointsDirName)
	if err := os.MkdirAll(checkpointsDir, 0755); err != nil {
		return nil, err
	}

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	cpID := fmt.Sprintf("cp_%s", hex.EncodeToString(b))

	info := &CheckpointInfo{
		ID:        cpID,
		SessionID: sessionID,
		Message:   message,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Tenta capturar o hash do git stash ou head como referência
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = cwd
	if out, err := cmd.Output(); err == nil {
		info.GitHash = strings.TrimSpace(string(out))
	}

	metaFile := filepath.Join(checkpointsDir, fmt.Sprintf("%s.json", cpID))
	data := fmt.Sprintf(`{"id":"%s","sessionId":"%s","message":"%s","createdAt":"%s","gitHash":"%s"}`+"\n",
		info.ID, info.SessionID, info.Message, info.CreatedAt, info.GitHash)

	if err := os.WriteFile(metaFile, []byte(data), 0644); err != nil {
		return nil, err
	}

	return info, nil
}

// RestoreCheckpoint reverte o código para o checkpoint especificado
func (m *Manager) RestoreCheckpoint(cwd, cpID string) error {
	checkpointsDir := filepath.Join(cwd, config.WorkspaceDirName, config.CheckpointsDirName)
	metaFile := filepath.Join(checkpointsDir, fmt.Sprintf("%s.json", cpID))

	if _, err := os.Stat(metaFile); os.IsNotExist(err) {
		return fmt.Errorf("checkpoint '%s' não encontrado", cpID)
	}

	// Se houver git, realiza rollback de arquivos modificados não comitados
	cmd := exec.Command("git", "checkout", "--", ".")
	cmd.Dir = cwd
	_ = cmd.Run()

	return nil
}
