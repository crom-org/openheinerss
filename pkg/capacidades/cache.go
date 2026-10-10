package capacidades

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/crom-org/openheinerss/pkg/config"
)

// EntradaCache guarda o que se viu da ajuda do CLI na última conferência de uma base.
type EntradaCache struct {
	Versao     string `json:"versao"`
	AjudaSHA   string `json:"ajudaSha256"`
	AjudaMudou bool   `json:"ajudaMudou,omitempty"`
	Em         string `json:"em"`
}

// ArquivoCache é <config do usuário>/openheinerss/capacidades-cache.json.
func ArquivoCache() (string, error) {
	dir, err := config.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openheinerss", "capacidades-cache.json"), nil
}

func lerCache() map[string]EntradaCache {
	m := map[string]EntradaCache{}
	if p, err := ArquivoCache(); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &m)
		}
	}
	return m
}

// HashAjuda é o sha256 do texto de ajuda do CLI.
func HashAjuda(ajuda string) string {
	h := sha256.Sum256([]byte(ajuda))
	return hex.EncodeToString(h[:])
}

// RegistrarAjuda grava a ajuda vista para a base e diz se ela mudou em relação à anterior
// (primeira vez não conta como mudança). Quando mudou, `capacidades` passa a avisar
// que as células devem ser reconferidas contra a versão nova.
func RegistrarAjuda(base, versao, ajuda string, agora time.Time) (mudou bool, err error) {
	p, err := ArquivoCache()
	if err != nil {
		return false, err
	}
	m := lerCache()
	novo := HashAjuda(ajuda)
	if antigo, ok := m[base]; ok && antigo.AjudaSHA != "" {
		mudou = antigo.AjudaSHA != novo
	}
	m[base] = EntradaCache{Versao: versao, AjudaSHA: novo, AjudaMudou: mudou, Em: agora.UTC().Format(time.RFC3339)}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return false, err
	}
	return mudou, os.Rename(tmp, p)
}

// DoCache devolve a entrada gravada para a base, se houver.
func DoCache(base string) (EntradaCache, bool) {
	e, ok := lerCache()[base]
	return e, ok
}
