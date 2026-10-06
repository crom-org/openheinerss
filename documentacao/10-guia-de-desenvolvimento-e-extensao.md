# 10 - Guia de Desenvolvimento & Como Criar Novos Harnesses

O Openheinerss foi concebido para ser infinitamente extensível. Qualquer desenvolvedor pode adicionar suporte a um novo motor de IA criando um novo pacote sob `pkg/harness/`.

---

## 1. Passo a Passo: Criando um Novo Adaptador de Harness

Para exemplificar, imagine criar um novo adaptador chamado `meu-motor`:

### Passo 1: Criar o Pacote em `pkg/harness/meumotor/meumotor.go`
```go
package meumotor

import (
	"context"
	"github.com/crom-org/openheinerss/pkg/harness"
)

type MeuMotorHarness struct {
	mode   harness.Mode
	events chan harness.Event
}

func NewMeuMotorHarness(mode harness.Mode) *MeuMotorHarness {
	return &MeuMotorHarness{
		mode:   mode,
		events: make(chan harness.Event, 100),
	}
}

func (h *MeuMotorHarness) Name() string {
	return "meu-motor"
}

func (h *MeuMotorHarness) Mode() harness.Mode {
	return h.mode
}

func (h *MeuMotorHarness) ValidatePrerequisites(ctx context.Context) harness.PrerequisiteResult {
	// Checar se binários ou chaves de API estão presentes
	return harness.PrerequisiteResult{Satisfied: true}
}

func (h *MeuMotorHarness) Start(ctx context.Context, cfg harness.SessionConfig) error {
	// Inicializar subprocesso ou cliente HTTP
	return nil
}

func (h *MeuMotorHarness) SendPrompt(ctx context.Context, text string, attachments []harness.Attachment) error {
	// Enviar comando para o motor e alimentar o canal h.events com eventos normalizados
	h.events <- harness.Event{
		Type:    harness.EventText,
		Payload: map[string]interface{}{"delta": "Processado pelo meu motor!"},
	}
	h.events <- harness.Event{
		Type:    harness.EventComplete,
		Payload: map[string]interface{}{"duration_ms": 100},
	}
	return nil
}

func (h *MeuMotorHarness) RespondPermission(ctx context.Context, reqID string, allow bool) error {
	// Retomar execução pausada após aprovação humana
	return nil
}

func (h *MeuMotorHarness) Events() <-chan harness.Event {
	return h.events
}

func (h *MeuMotorHarness) Stop() error {
	close(h.events)
	return nil
}
```

---

### Passo 2: Registrar o Motor em `cmd/openheinerss/main.go`
Adicione o seu construtor ao registro global:
```go
registry.Register("meu-motor", func(mode harness.Mode) (harness.Harness, error) {
    return meumotor.NewMeuMotorHarness(mode), nil
})
```

---

### Passo 3: Escrever Testes Unitários (`meumotor_test.go`)
```go
package meumotor

import (
	"testing"
	"github.com/crom-org/openheinerss/pkg/harness"
)

func TestMeuMotorRegistration(t *testing.T) {
	h := NewMeuMotorHarness(harness.ModeCLI)
	if h.Name() != "meu-motor" {
		t.Fatalf("esperado meu-motor, obtido %s", h.Name())
	}
}
```

---

## 2. Comandos do Makefile para Desenvolvedores

O repositório inclui utilitários de automação:

```bash
# Executar todos os testes unitários com saída verbosa:
make test

# Compilar o binário em ./openheinerss:
make build

# Instalar localmente em ~/.local/bin/openheinerss:
make install

# Limpar binários temporários:
make clean
```

---

## 3. Padrões de Contribuição na Organização `crom-org`

1. **Zero Quebra de Compatibilidade no Protocolo**: Qualquer alteração em mensagens JSON-RPC 2.0 deve manter retrocompatibilidade com os SDKs de TypeScript, PHP e Python.
2. **Tratamento Seguro de Subprocessos**: Nenhum processo filho deve permanecer como processo zumbi (*orphan process*). Todos os adaptadores devem propagar sinais de terminação limpos ao receber `Stop()` ou cancelamento de contexto `context.Context`.
3. **Erros Acionáveis**: Sempre forneça a propriedade `suggestedFix` em falhas de pré-requisitos para que o usuário saiba exatamente como resolver a pendência no terminal.
