package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
	"github.com/crom-org/openheinerss/pkg/protocol"
	"github.com/spf13/cobra"
)

type harnessTestCase struct {
	Name string
	Base string
	Mode harness.Mode
}

type harnessTestResult struct {
	Nome      string `json:"nome"`
	Base      string `json:"base"`
	Modo      string `json:"modo"`
	Resultado string `json:"resultado"`
	TempoMS   int64  `json:"tempo_ms"`
	Tokens    int64  `json:"tokens"`
	Motivo    string `json:"motivo,omitempty"`
}

func harnessTestCases(includePrincipal bool) []harnessTestCase {
	// A lista vem do catálogo (embutidos + instâncias do usuário); nada de instância fixa no código.
	// O mock fica de fora (não é motor real) e o claude-code puro só entra com --incluir-principal.
	var items []harnessTestCase
	for _, item := range harness.ListCatalog() {
		base := item.ID
		if spec, ok := harness.CustomSpecFor(item.ID); ok && spec.Base != "" {
			base = rootBase(spec.Base)
		}
		if item.ID == "mock" || base == "mock" || (item.ID == "claude-code" && !includePrincipal) {
			continue
		}
		items = append(items, harnessTestCase{Name: item.ID, Base: base, Mode: harness.ModeCLI})
		if base == "claude-code" {
			items = append(items, harnessTestCase{Name: item.ID, Base: base, Mode: harness.ModeSDK})
		}
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].Name < items[b].Name })
	return items
}

// rootBase segue a cadeia de herança até o harness embutido.
func rootBase(name string) string {
	for i := 0; i < 10; i++ {
		spec, ok := harness.CustomSpecFor(name)
		if !ok || spec.Base == "" {
			return name
		}
		name = spec.Base
	}
	return name
}

func runHarnessTodos(cmd *cobra.Command, prompt string, jsonOutput bool, skip string, timeout time.Duration, includePrincipal bool) error {
	if prompt == "" {
		prompt = "responda só OK"
	}
	if timeout <= 0 {
		return fmt.Errorf("--timeout deve ser positivo")
	}
	skips := map[string]bool{}
	for _, item := range strings.Split(skip, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			skips[item] = true
		}
	}
	var results []harnessTestResult
	for _, tc := range harnessTestCases(includePrincipal) {
		key := tc.Name
		modeKey := key + ":" + string(tc.Mode)
		if skips[key] || skips[modeKey] {
			continue
		}
		results = append(results, runHarnessTest(cmd.Context(), tc, prompt, timeout))
	}
	if jsonOutput {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Println("nome | base | modo | resultado | tempo | tokens | motivo")
	for _, r := range results {
		fmt.Printf("%s | %s | %s | %s | %s | %d | %s\n", r.Nome, r.Base, r.Modo, r.Resultado, formatDuration(r.TempoMS), r.Tokens, r.Motivo)
	}
	return nil
}

func runHarnessTest(parent context.Context, tc harnessTestCase, prompt string, timeout time.Duration) harnessTestResult {
	started := time.Now()
	result := harnessTestResult{Nome: tc.Name, Base: tc.Base, Modo: string(tc.Mode)}
	h, err := harness.Create(tc.Name, tc.Mode)
	if err != nil {
		result.Resultado, result.Motivo = "falha", err.Error()
		result.TempoMS = time.Since(started).Milliseconds()
		return result
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if prereq := h.ValidatePrerequisites(ctx); !prereq.Satisfied {
		result.Resultado = "sem CLI"
		result.Motivo = strings.Join(prereq.MissingItems, ", ")
		if prereq.SuggestedFix != "" {
			result.Motivo += ": " + prereq.SuggestedFix
		}
		result.TempoMS = time.Since(started).Milliseconds()
		return result
	}
	cwd := "."
	if err := h.Start(ctx, harness.SessionConfig{SessionID: "harness-test", CWD: cwd}); err != nil {
		result.Resultado, result.Motivo = classifyHarnessFailure(err.Error())
		result.TempoMS = time.Since(started).Milliseconds()
		return result
	}
	defer stopHarnessWithLimit(h)
	if err := h.SendPrompt(ctx, prompt, nil); err != nil {
		result.Resultado, result.Motivo = classifyHarnessFailure(err.Error())
		result.TempoMS = time.Since(started).Milliseconds()
		return result
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
					result.Tokens += usage.TotalTokens
					if result.Tokens == 0 {
						result.Tokens = usage.InputTokens + usage.OutputTokens
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
			case harness.EventComplete:
				gotComplete = true
			}
		case <-ctx.Done():
			result.Resultado, result.Motivo = "falha", "tempo esgotado"
			result.TempoMS = time.Since(started).Milliseconds()
			return result
		}
	}
	if failure != "" {
		result.Resultado, result.Motivo = classifyHarnessFailure(failure)
	} else if !gotText {
		result.Resultado, result.Motivo = "falha", "o motor terminou sem texto"
	} else {
		result.Resultado, result.Motivo = "OK", "texto e fim de sucesso confirmados"
	}
	result.TempoMS = time.Since(started).Milliseconds()
	return result
}

func stopHarnessWithLimit(h harness.Harness) {
	done := make(chan struct{})
	go func() {
		_ = h.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// O resultado do teste já está definido; não bloqueie a matriz por
		// um CLI que não liberou seus pipes depois do timeout.
	}
}

func classifyHarnessFailure(message string) (string, string) {
	kind, _ := harness.ClassifyFailure(message)
	return kind, message
}

func formatDuration(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(time.Millisecond).String()
}

func runHarnessResume(parent context.Context, name, prompt string, timeout time.Duration) error {
	if prompt == "" {
		prompt = "responda só OK"
	}
	h, err := harness.Create(name, harness.ModeCLI)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	defer stopHarnessWithLimit(h)
	if prereq := h.ValidatePrerequisites(ctx); !prereq.Satisfied {
		return fmt.Errorf("sem CLI: %s", strings.Join(prereq.MissingItems, ", "))
	}
	if err := h.Start(ctx, harness.SessionConfig{SessionID: "harness-resume", CWD: "."}); err != nil {
		return err
	}
	if err := h.SendPrompt(ctx, prompt, nil); err != nil {
		return err
	}
	for turn := 1; turn <= 2; turn++ {
		gotText := false
		for {
			select {
			case event := <-h.Events():
				switch event.Type {
				case harness.EventText:
					if p, ok := event.Payload.(protocol.TextParams); ok && strings.TrimSpace(p.Delta) != "" {
						gotText = true
					}
				case harness.EventError:
					if p, ok := event.Payload.(protocol.ErrorParams); ok {
						return fmt.Errorf("%s: %s", classifyLabel(p.Message), p.Message)
					}
				case harness.EventComplete:
					if !gotText {
						return fmt.Errorf("turno %d terminou sem texto", turn)
					}
					fmt.Printf("turno %d OK\n", turn)
					if turn == 1 {
						if err := h.SendPrompt(ctx, "qual palavra você respondeu antes?", nil); err != nil {
							return err
						}
					}
					goto nextTurn
				}
			case <-ctx.Done():
				return fmt.Errorf("tempo esgotado")
			}
		}
	nextTurn:
	}
	return nil
}

func classifyLabel(message string) string {
	result, _ := classifyHarnessFailure(message)
	return result
}
