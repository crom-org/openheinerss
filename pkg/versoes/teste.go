package versoes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
)

// ResultadoTeste é a saída de TesteReal (a mesma de `harness test`).
type ResultadoTeste struct {
	Resultado string // OK | falha | sem CLI | classificações de harness.ClassifyFailure
	Motivo    string
	Tokens    int64
	TempoMS   int64
}

// TesteReal envia um prompt curto ao harness/instância e confere texto + fim de sucesso.
func TesteReal(parent context.Context, nome string, modo harness.Mode, prompt string, timeout time.Duration) ResultadoTeste {
	started := time.Now()
	var res ResultadoTeste
	defer func() { res.TempoMS = time.Since(started).Milliseconds() }()
	falha := func(msg string) ResultadoTeste {
		kind, _ := harness.ClassifyFailure(msg)
		res.Resultado, res.Motivo = kind, msg
		return res
	}
	h, err := harness.Create(nome, modo)
	if err != nil {
		res.Resultado, res.Motivo = "falha", err.Error()
		return res
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if prereq := h.ValidatePrerequisites(ctx); !prereq.Satisfied {
		res.Resultado = "sem CLI"
		res.Motivo = strings.Join(prereq.MissingItems, ", ")
		if prereq.SuggestedFix != "" {
			res.Motivo += ": " + prereq.SuggestedFix
		}
		return res
	}
	if err := h.Start(ctx, harness.SessionConfig{SessionID: "harness-test", CWD: "."}); err != nil {
		return falha(err.Error())
	}
	defer parar(h)
	if err := h.SendPrompt(ctx, prompt, nil); err != nil {
		return falha(err.Error())
	}
	gotText, gotComplete := false, false
	var failure string
	for !gotComplete {
		select {
		case event := <-h.Events():
			switch event.Type {
			case harness.EventText:
				if text, ok := event.Payload.(protocol.TextParams); ok && strings.TrimSpace(text.Delta) != "" {
					gotText = true
				}
			case harness.EventUsage:
				if usage, ok := event.Payload.(protocol.UsageParams); ok {
					res.Tokens += usage.TotalTokens
					if res.Tokens == 0 {
						res.Tokens = usage.InputTokens + usage.OutputTokens
					}
				}
			case harness.EventError:
				if failure == "" {
					if e, ok := event.Payload.(protocol.ErrorParams); ok {
						failure = e.Message
					} else {
						failure = fmt.Sprint(event.Payload)
					}
				}
			case harness.EventPermission:
				if p, ok := event.Payload.(protocol.PermissionRequestParams); ok {
					_ = h.RespondPermission(ctx, p.RequestID, false, "harness test não autoriza ferramentas")
				}
			case harness.EventComplete:
				gotComplete = true
			}
		case <-ctx.Done():
			res.Resultado, res.Motivo = "falha", "tempo esgotado"
			return res
		}
	}
	switch {
	case failure != "":
		return falha(failure)
	case !gotText:
		res.Resultado, res.Motivo = "falha", "o motor terminou sem texto"
	default:
		res.Resultado, res.Motivo = "OK", "texto e fim de sucesso confirmados"
	}
	return res
}

func parar(h harness.Harness) {
	done := make(chan struct{})
	go func() { _ = h.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
