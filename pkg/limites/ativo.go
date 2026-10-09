package limites

// Consulta ativa de limites. Este arquivo deliberadamente não expõe nem persiste credenciais.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AtualizarOpcoes struct {
	Forcar     bool
	Intervalo  time.Duration
	HTTPClient *http.Client
	CacheDir   string
}

var atualizaMu sync.Mutex

// Atualizar consulta as contas locais respeitando o intervalo mínimo por conta.
func Atualizar(ctx context.Context, op AtualizarOpcoes) (Resultado, error) {
	if op.Intervalo <= 0 {
		op.Intervalo = 5 * time.Minute
	}
	if op.HTTPClient == nil {
		op.HTTPClient = http.DefaultClient
	}
	base := Obter()
	cacheDir := op.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(userHome(), ".cache", "openheinerss", "limites")
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return base, err
	}
	resultado := base
	for n := range resultado.Instancias {
		i := &resultado.Instancias[n]
		if i.Base != "claude-code" && i.Base != "codex" {
			continue
		}
		if !op.Forcar {
			if c, ok := lerCache(filepath.Join(cacheDir, nomeCache(i.Nome)), time.Now(), op.Intervalo); ok {
				*i = mesclar(*i, c)
				continue
			}
		}
		var got Instancia
		var err error
		if i.Base == "claude-code" {
			got, err = consultaClaude(ctx, *i, op.HTTPClient)
		} else {
			got, err = consultaCodex(ctx, *i, op.HTTPClient)
		}
		if err != nil {
			// O dado passivo continua útil, mas o erro não inclui URL nem credencial.
			i.Nota = "consulta ativa indisponível: " + erroSeguro(err)
			continue
		}
		*i = mesclar(*i, got)
		_ = gravarCache(filepath.Join(cacheDir, nomeCache(i.Nome)), got)
	}
	resultado.Agora = time.Now().Format(time.RFC3339)
	return resultado, nil
}

func mesclar(antigo, novo Instancia) Instancia {
	if novo.Nome == "" {
		novo.Nome = antigo.Nome
	}
	if novo.Base == "" {
		novo.Base = antigo.Base
	}
	return novo
}
func nomeCache(nome string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(nome) + ".json"
}
func gravarCache(path string, i Instancia) error {
	atualizaMu.Lock()
	defer atualizaMu.Unlock()
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err = os.Chmod(tmp, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func lerCache(path string, agora time.Time, intervalo time.Duration) (Instancia, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Instancia{}, false
	}
	var i Instancia
	if json.Unmarshal(b, &i) != nil || i.DadoEm == "" {
		return Instancia{}, false
	}
	t, err := time.Parse(time.RFC3339, i.DadoEm)
	if err != nil || agora.Sub(t) >= intervalo {
		return Instancia{}, false
	}
	i.IdadeSegundos = int64(agora.Sub(t).Seconds())
	return i, true
}

func tokenArquivo(path string, chaves ...string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var v interface{}
	if json.Unmarshal(b, &v) != nil {
		return "", errors.New("credencial inválida")
	}
	return acharToken(v, chaves...), nil
}
func acharToken(v interface{}, chaves ...string) string {
	if m, ok := v.(map[string]interface{}); ok {
		for _, chave := range chaves {
			if x, ok := m[chave].(string); ok && x != "" {
				return x
			}
		}
		for _, x := range m {
			if t := acharToken(x, chaves...); t != "" {
				return t
			}
		}
	}
	if a, ok := v.([]interface{}); ok {
		for _, x := range a {
			if t := acharToken(x, chaves...); t != "" {
				return t
			}
		}
	}
	return ""
}
func requestJSON(ctx context.Context, client *http.Client, url, token string, headers map[string]string) (map[string]interface{}, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.New("rede indisponível")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, res.Header, errors.New("resposta inválida")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, res.Header, fmt.Errorf("servidor respondeu HTTP %d", res.StatusCode)
	}
	var v map[string]interface{}
	if json.Unmarshal(b, &v) != nil {
		return nil, res.Header, errors.New("resposta inválida")
	}
	return v, res.Header, nil
}
func consultaClaude(ctx context.Context, base Instancia, client *http.Client) (Instancia, error) {
	dir := dirClaude(base)
	token, err := tokenArquivo(filepath.Join(dir, ".credentials.json"), "accessToken", "oauthAccessToken", "token")
	if err != nil || token == "" {
		return base, errors.New("credencial não encontrada")
	}
	url := os.Getenv("OPENHEINERSS_CLAUDE_USAGE_URL")
	if url == "" {
		url = "https://api.anthropic.com/api/oauth/usage"
	}
	v, headers, err := requestJSON(ctx, client, url, token, map[string]string{"anthropic-beta": "oauth-2025-04-20"})
	if err != nil {
		return base, err
	}
	i := instanciaUso(base, v, headers, "consulta-ativa")
	return i, nil
}
func consultaCodex(ctx context.Context, base Instancia, client *http.Client) (Instancia, error) {
	home := dirCodex(base)
	token, err := tokenArquivo(filepath.Join(home, "auth.json"), "access_token", "accessToken", "token")
	if err != nil || token == "" {
		return base, errors.New("credencial não encontrada")
	}
	url := os.Getenv("OPENHEINERSS_CODEX_USAGE_URL")
	if url == "" {
		url = "https://chatgpt.com/backend-api/wham/usage"
	}
	v, headers, err := requestJSON(ctx, client, url, token, map[string]string{"ChatGPT-Account-Id": ""})
	if err != nil {
		return base, err
	}
	return instanciaUso(base, v, headers, "consulta-ativa"), nil
}
func instanciaUso(base Instancia, v map[string]interface{}, h http.Header, fonte string) Instancia {
	i := Instancia{Nome: base.Nome, Base: base.Base, DadoEm: time.Now().Format(time.RFC3339), Fonte: fonte}
	if h.Get("anthropic-ratelimit-unified-5h-utilization") != "" {
		i.Fonte = "cabeçalhos"
		i.Janelas = append(i.Janelas, Janela{Nome: "5 h", Percentual: parseFloat(h.Get("anthropic-ratelimit-unified-5h-utilization")) * 100})
	}
	if h.Get("anthropic-ratelimit-unified-7d-utilization") != "" {
		i.Fonte = "cabeçalhos"
		i.Janelas = append(i.Janelas, Janela{Nome: "semanal", Percentual: parseFloat(h.Get("anthropic-ratelimit-unified-7d-utilization")) * 100})
	}
	walkUso(v, &i)
	return i
}
func walkUso(v interface{}, i *Instancia) {
	if m, ok := v.(map[string]interface{}); ok {
		for k, x := range m {
			lk := strings.ToLower(k)
			if lk == "used_percent" || lk == "used_percentage" || lk == "utilization" {
				if f, ok := num(x); ok {
					if f <= 1 {
						f *= 100
					}
					i.Janelas = append(i.Janelas, Janela{Nome: "limite", Percentual: f})
				}
			}
			walkUso(x, i)
		}
	}
	if a, ok := v.([]interface{}); ok {
		for _, x := range a {
			walkUso(x, i)
		}
	}
}
func num(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case json.Number:
		f, e := x.Float64()
		return f, e == nil
	}
	return 0, false
}
func parseFloat(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }
func dirClaude(i Instancia) string {
	if i.Nome == "claude-conta2" {
		return filepath.Join(userHome(), ".claude-conta2")
	}
	if strings.HasPrefix(i.Nome, "claude-") && i.Nome != "claude-code" {
		return filepath.Join(userHome(), "."+i.Nome)
	}
	return expandHome(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(userHome(), ".claude"))
}
func dirCodex(i Instancia) string {
	if i.Nome == "codex2" {
		return filepath.Join(userHome(), ".codex-compartilhado")
	}
	return expandHome(os.Getenv("CODEX_HOME"), filepath.Join(userHome(), ".codex"))
}
func erroSeguro(err error) string {
	s := err.Error()
	if strings.Contains(strings.ToLower(s), "token") || strings.Contains(strings.ToLower(s), "bearer") {
		return "autenticação recusada"
	}
	return s
}
