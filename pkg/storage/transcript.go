package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// TranscriptEntry representa uma linha gravada no arquivo de histórico de sessão (.jsonl)
type TranscriptEntry struct {
	Timestamp string                 `json:"timestamp"`
	SessionID string                 `json:"sessionId"`
	Type      string                 `json:"type"` // "user_prompt", "event", "system"
	Event     *protocol.Notification `json:"event,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Config    interface{}            `json:"config,omitempty"`
}

// Storage coordena a gravação e leitura de históricos de sessões
type Storage struct {
	mu sync.Mutex
}

var defaultStorage = &Storage{}

// GetStorage retorna a instância padrão de armazenamento
func GetStorage() *Storage {
	return defaultStorage
}

// RecordEvent adiciona um evento no arquivo .jsonl da sessão
func (s *Storage) RecordEvent(cwd, sessionID, entryType string, notification *protocol.Notification, prompt string) error {
	return s.Record(cwd, sessionID, entryType, notification, prompt, nil)
}

// idValido recusa IDs que saem da pasta de sessões (o ID vem do cliente em session.resume).
func idValido(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`+"\x00")
}

func (s *Storage) Record(cwd, sessionID, entryType string, notification *protocol.Notification, prompt string, sessionConfig interface{}) error {
	if !idValido(sessionID) {
		return fmt.Errorf("id de sessão inválido: %q", sessionID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionsDir := filepath.Join(cwd, config.WorkspaceDirName, config.SessionsDirName)
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(sessionsDir, fmt.Sprintf("%s.jsonl", sessionID))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := TranscriptEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		SessionID: sessionID,
		Type:      entryType,
		Event:     notification,
		Prompt:    prompt,
		Config:    sessionConfig,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// LoadSession lê o histórico completo de uma sessão gravada
func (s *Storage) LoadSession(cwd, sessionID string) ([]TranscriptEntry, error) {
	if !idValido(sessionID) {
		return nil, fmt.Errorf("id de sessão inválido: %q", sessionID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(cwd, config.WorkspaceDirName, config.SessionsDirName, fmt.Sprintf("%s.jsonl", sessionID))
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("sessão '%s' não encontrada em disco: %w", sessionID, err)
	}
	defer f.Close()

	var entries []TranscriptEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		var entry TranscriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entries = append(entries, entry)
		}
	}

	return entries, scanner.Err()
}

// ListPersistedSessions lista todas as sessões armazenadas na pasta .openheinerss/sessions
func (s *Storage) ListPersistedSessions(cwd string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionsDir := filepath.Join(cwd, config.WorkspaceDirName, config.SessionsDirName)
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var sessionIDs []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
			sessionIDs = append(sessionIDs, e.Name()[:len(e.Name())-6])
		}
	}

	return sessionIDs, nil
}
