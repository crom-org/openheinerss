package storage_test

import (
	"os"
	"testing"

	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/crom-org/openheinerss/pkg/storage"
)

func TestStoragePersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "openheinerss_storage_test_*")
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s := storage.GetStorage()
	sessID := "sess_test_persist_123"

	// 1. Grava prompt
	if err := s.RecordEvent(tempDir, sessID, "user_prompt", nil, "refatore o código"); err != nil {
		t.Fatalf("falha ao gravar prompt: %v", err)
	}

	// 2. Grava evento
	notif := protocol.NewNotification(protocol.EventAgentText, protocol.TextParams{
		SessionID: sessID,
		Delta:     "Criando arquivo...",
	})
	if err := s.RecordEvent(tempDir, sessID, "event", &notif, ""); err != nil {
		t.Fatalf("falha ao gravar evento: %v", err)
	}

	// 3. Lê entradas
	entries, err := s.LoadSession(tempDir, sessID)
	if err != nil {
		t.Fatalf("falha ao carregar sessão: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperava 2 entradas, obteve %d", len(entries))
	}
	if entries[0].Prompt != "refatore o código" {
		t.Errorf("prompt incorreto: %s", entries[0].Prompt)
	}

	// 4. Lista sessões
	list, err := s.ListPersistedSessions(tempDir)
	if err != nil {
		t.Fatalf("falha ao listar sessões: %v", err)
	}
	if len(list) != 1 || list[0] != sessID {
		t.Fatalf("listagem incorreta: %v", list)
	}
}

func TestIDDeSessaoNaoSaiDaPasta(t *testing.T) {
	s := storage.GetStorage()
	dir := t.TempDir()
	for _, id := range []string{"../fora", "a/b", "..", ""} {
		if _, err := s.LoadSession(dir, id); err == nil {
			t.Errorf("LoadSession aceitou %q", id)
		}
		if err := s.Record(dir, id, "system", nil, "", nil); err == nil {
			t.Errorf("Record aceitou %q", id)
		}
	}
}
