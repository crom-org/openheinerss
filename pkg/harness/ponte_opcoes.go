package harness

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

// OpcaoLista lê uma lista de textos de options (aceita []string, []interface{} e string).
func OpcaoLista(options map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		switch v := options[key].(type) {
		case []string:
			return append([]string(nil), v...)
		case []interface{}:
			out := make([]string, 0, len(v))
			for _, item := range v {
				out = append(out, fmt.Sprint(item))
			}
			return out
		case string:
			if v != "" {
				return []string{v}
			}
		}
	}
	return nil
}

// OpcaoTexto lê o primeiro texto não vazio entre as chaves de options.
func OpcaoTexto(options map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := options[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// OpcaoBool lê um booleano de options (aceita bool e "true"/"1"/"sim").
func OpcaoBool(options map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		switch v := options[key].(type) {
		case bool:
			if v {
				return true
			}
		case string:
			switch strings.ToLower(v) {
			case "true", "1", "sim", "yes":
				return true
			}
		}
	}
	return false
}

// AnexosEmArquivos grava os anexos (base64) em arquivos temporários para harnesses que só
// aceitam caminhos (--file). Devolve os caminhos e a função que apaga a pasta temporária.
func AnexosEmArquivos(attachments []protocol.Attachment) ([]string, func(), error) {
	if len(attachments) == 0 {
		return nil, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "openheinerss-anexos-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	var paths []string
	for i, a := range attachments {
		data, err := base64.StdEncoding.DecodeString(a.Data)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("anexo %d: base64 inválido: %w", i+1, err)
		}
		ext := ".bin"
		if _, sub, ok := strings.Cut(a.MediaType, "/"); ok && sub != "" {
			ext = "." + strings.Map(func(r rune) rune {
				if r == '+' || r == ';' || r == ' ' {
					return '-'
				}
				return r
			}, sub)
		}
		path := filepath.Join(dir, fmt.Sprintf("anexo-%d%s", i+1, ext))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		paths = append(paths, path)
	}
	return paths, cleanup, nil
}

// LineWriter divide o que recebe em linhas e entrega cada uma a fn (usado para o stderr virar raw).
type LineWriter struct {
	mu  sync.Mutex
	buf []byte
	fn  func(line string)
}

// NewLineWriter cria um LineWriter; chame Flush ao fim para entregar a última linha sem "\n".
func NewLineWriter(fn func(line string)) *LineWriter { return &LineWriter{fn: fn} }

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf = append(w.buf, p...)
	var lines []string
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	w.mu.Unlock()
	for _, l := range lines {
		if l != "" {
			w.fn(l)
		}
	}
	return len(p), nil
}

// Flush entrega o resto pendente.
func (w *LineWriter) Flush() {
	w.mu.Lock()
	rest := strings.TrimRight(string(w.buf), "\r\n")
	w.buf = nil
	w.mu.Unlock()
	if rest != "" {
		w.fn(rest)
	}
}
