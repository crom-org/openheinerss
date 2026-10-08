package harness

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

// O openheinerss é só a ponte: o que o harness oferece precisa chegar a ele sem filtro.
// Este arquivo reúne as convenções comuns a todos os adaptadores.

// OptionHarnessArgs é a chave de SessionConfig.Options com os argumentos nativos extras
// (--arg/--harness-arg na CLI, harnessArgs no protocolo). Eles vão intactos e na mesma
// ordem ao processo do harness, antes do prompt.
const OptionHarnessArgs = "harness_args"

// EventRaw carrega uma linha original do harness que não tem mapeamento para outro evento.
const EventRaw EventType = "raw"

// HarnessArgs devolve os argumentos nativos extras guardados em options.
// Aceita []string (Go) e []interface{} (JSON decodificado).
func HarnessArgs(options map[string]interface{}) []string {
	return OpcaoLista(options, OptionHarnessArgs)
}

var slashName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]*$`)

// SlashCommand reconhece um comando "/nome args". Caminhos como "/home/x" não contam.
func SlashCommand(text string) (name, rest string, ok bool) {
	t := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(t, "/") {
		return "", "", false
	}
	t = t[1:]
	name, rest, _ = strings.Cut(t, " ")
	if i := strings.IndexAny(name, "\t\n"); i >= 0 {
		rest = name[i+1:] + " " + rest
		name = name[:i]
	}
	if !slashName.MatchString(name) {
		return "", "", false
	}
	return name, strings.TrimSpace(rest), true
}

// NoEquivalentError diz claramente que o harness não tem como receber o comando no modo sem tela.
type NoEquivalentError struct {
	Harness, Command, Hint string
}

func (e *NoEquivalentError) Error() string {
	msg := fmt.Sprintf("o harness %s não aceita /%s no modo sem tela e não há equivalente na linha de comando", e.Harness, e.Command)
	if e.Hint != "" {
		msg += "; " + e.Hint
	}
	return msg
}

// NoEquivalent cria o erro padrão para comando sem equivalente.
func NoEquivalent(harnessName, command, hint string) error {
	return &NoEquivalentError{Harness: harnessName, Command: command, Hint: hint}
}

// RawEvent embala uma linha original do harness (stream "stdout" ou "stderr").
func RawEvent(sessionID, harnessName, stream, line string) Event {
	return Event{Type: EventRaw, Payload: protocol.RawParams{SessionID: sessionID, Harness: harnessName, Stream: stream, Line: line}}
}

// OptionStrings devolve uma opção do tipo lista de textos. Aceita []string, []interface{}
// e um texto único (JSON decodificado ou valor da CLI).
func OptionStrings(options map[string]interface{}, key string) []string {
	return OpcaoLista(options, key)
}

// WithOption devolve uma cópia de options com key=value, sem alterar o mapa original.
func WithOption(options map[string]interface{}, key string, value interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(options)+1)
	for k, v := range options {
		out[k] = v
	}
	out[key] = value
	return out
}
