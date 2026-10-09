package limites

// Consulta ativa de limites. Este arquivo deliberadamente não expõe nem persiste credenciais.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crom-org/openheinerss/pkg/harness"
)

type AtualizarOpcoes struct {
	Forcar     bool
	Intervalo  time.Duration
	HTTPClient *http.Client
	CacheDir   string
}

const claudeOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

const intervaloReservaClaude = 5 * time.Minute

type httpStatusError struct {
	status int
	header http.Header
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("servidor respondeu HTTP %d", e.status) }

var atualizaMu sync.Mutex

// Atualizar consulta as contas locais. Com Forcar, a consulta é sempre feita na
// hora e ignora tanto o cache quanto os bloqueios de retry; Forcar é mantido
// como compatibilidade com --forcar.
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
		cachePath := filepath.Join(cacheDir, nomeCache(i.Nome))
		unlock, lockErr := travarConta(cachePath + ".lock")
		if lockErr != nil {
			i.Nota = "consulta ativa indisponível: trava de cache"
			continue
		}
		func() {
			defer unlock()
			if !op.Forcar {
				if retryAte, ok := lerRetryAte(retryPath(cachePath)); ok && time.Now().Before(retryAte) {
					if i.Base == "claude-code" {
						// O bloqueio do endpoint de uso não bloqueia a reserva mínima.
						// Ela tem seu próprio intervalo, compartilhado por processos via
						// cache/lock, para não repetir uma mensagem a cada atualização.
						if c, cacheOK := lerCache(cachePath, time.Now(), intervaloReservaClaude); cacheOK && c.Fonte == "cabeçalhos" {
							*i = mesclar(*i, c)
							return
						}
						if reservaAte, reservaBloqueada := lerRetryAte(reservaRetryPath(cachePath)); reservaBloqueada && time.Now().Before(reservaAte) {
							aplicarFalha429(i, cachePath, retryAte, fmt.Sprintf("intervalo mínimo da reserva até %s", reservaAte.Format(time.RFC3339)))
							return
						}
						_ = gravarRetryAte(reservaRetryPath(cachePath), time.Now().Add(intervaloReservaClaude))
						reservado, reservaErr := reservaClaude(ctx, *i, op.HTTPClient)
						if reservaErr == nil && len(reservado.Janelas) > 0 {
							reservado.Nota = notaReserva429()
							*i = mesclar(*i, reservado)
							_ = gravarCache(cachePath, reservado)
							return
						}
						motivo := "reserva sem cabeçalhos de limite"
						if reservaErr != nil {
							motivo = erroSeguro(reservaErr)
						}
						aplicarFalha429(i, cachePath, retryAte, motivo)
						return
					}
					if c, ok := lerCacheSemValidade(cachePath); ok {
						c.Fonte = "cache (429)"
						c.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s", retryAte.Format(time.RFC3339))
						atualizarIdade(&c, time.Now())
						*i = mesclar(*i, c)
					} else {
						i.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s", retryAte.Format(time.RFC3339))
					}
					return
				}
			}
			if !op.Forcar {
				if c, ok := lerCache(cachePath, time.Now(), op.Intervalo); ok {
					*i = mesclar(*i, c)
					return
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
				if he, ok := err.(*httpStatusError); ok && he.status == http.StatusTooManyRequests {
					retryAte := time.Now().Add(retryAfter(he.header.Get("Retry-After")))
					_ = gravarRetryAte(retryPath(cachePath), retryAte)
					reservaMotivo := "reserva sem cabeçalhos de limite"
					if i.Base == "claude-code" {
						if !op.Forcar {
							if reservaAte, reservaBloqueada := lerRetryAte(reservaRetryPath(cachePath)); reservaBloqueada && time.Now().Before(reservaAte) {
								reservaMotivo = fmt.Sprintf("intervalo mínimo da reserva até %s", reservaAte.Format(time.RFC3339))
							} else {
								_ = gravarRetryAte(reservaRetryPath(cachePath), time.Now().Add(intervaloReservaClaude))
								reservado, reservaErr := reservaClaude(ctx, *i, op.HTTPClient)
								if reservaErr == nil && len(reservado.Janelas) > 0 {
									reservado.Nota = notaReserva429()
									*i = mesclar(*i, reservado)
									_ = gravarCache(cachePath, reservado)
									return
								}
								if reservaErr != nil {
									reservaMotivo = erroSeguro(reservaErr)
								}
							}
						} else {
							reservado, reservaErr := reservaClaude(ctx, *i, op.HTTPClient)
							if reservaErr == nil && len(reservado.Janelas) > 0 {
								reservado.Nota = notaReserva429()
								*i = mesclar(*i, reservado)
								_ = gravarCache(cachePath, reservado)
								return
							}
							if reservaErr != nil {
								reservaMotivo = erroSeguro(reservaErr)
							}
						}
					}
					if c, cacheOK := lerCacheSemValidade(cachePath); cacheOK {
						c.Fonte = "cache (429)"
						c.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s", retryAte.Format(time.RFC3339))
						if reservaMotivo != "" {
							c.Nota += "; reserva de cabeçalhos falhou: " + reservaMotivo
						}
						atualizarIdade(&c, time.Now())
						*i = mesclar(*i, c)
					} else {
						i.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s", retryAte.Format(time.RFC3339))
						if reservaMotivo != "" {
							i.Nota += "; reserva de cabeçalhos falhou: " + reservaMotivo
						}
					}
					return
				}
				// O dado passivo continua útil, mas o erro não inclui URL nem credencial.
				i.Nota = "consulta ativa indisponível: " + erroSeguro(err)
				return
			}
			*i = mesclar(*i, got)
			_ = gravarCache(cachePath, got)
		}()
	}
	resultado.Agora = time.Now().Format(time.RFC3339)
	return resultado, nil
}

func notaReserva429() string {
	return "/api/oauth/usage respondeu HTTP 429; reserva mínima (Haiku 4.5, max_tokens=1), custo: 1 token de saída mais tokens de entrada"
}

func aplicarFalha429(i *Instancia, cachePath string, retryAte time.Time, reservaMotivo string) {
	if c, cacheOK := lerCacheSemValidade(cachePath); cacheOK {
		c.Fonte = "cache (429)"
		c.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s; reserva de cabeçalhos falhou: %s", retryAte.Format(time.RFC3339), reservaMotivo)
		atualizarIdade(&c, time.Now())
		*i = mesclar(*i, c)
		return
	}
	i.Nota = fmt.Sprintf("HTTP 429; nova tentativa após %s; reserva de cabeçalhos falhou: %s", retryAte.Format(time.RFC3339), reservaMotivo)
}

func mesclar(antigo, novo Instancia) Instancia {
	if novo.Nome == "" {
		novo.Nome = antigo.Nome
	}
	if novo.Base == "" {
		novo.Base = antigo.Base
	}
	novo.Janelas = janelasVigentes(novo.Janelas, time.Now())
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
	i, ok := lerCacheSemValidade(path)
	if !ok {
		return Instancia{}, false
	}
	if len(janelasVigentes(i.Janelas, agora)) == 0 {
		return Instancia{}, false
	}
	t, err := time.Parse(time.RFC3339, i.DadoEm)
	if err != nil || agora.Sub(t) >= intervalo {
		return Instancia{}, false
	}
	atualizarIdade(&i, agora)
	return i, true
}

func atualizarIdade(i *Instancia, agora time.Time) {
	t, err := time.Parse(time.RFC3339, i.DadoEm)
	if err != nil {
		return
	}
	i.IdadeSegundos = int64(agora.Sub(t).Seconds())
}

func lerCacheSemValidade(path string) (Instancia, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Instancia{}, false
	}
	var i Instancia
	if json.Unmarshal(b, &i) != nil || i.DadoEm == "" {
		return Instancia{}, false
	}
	if i.DadoEm == "" {
		return Instancia{}, false
	}
	return i, true
}

func retryPath(cachePath string) string { return cachePath + ".retry" }

func reservaRetryPath(cachePath string) string { return cachePath + ".reserva.retry" }

func lerRetryAte(path string) (time.Time, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	return t, err == nil
}

func gravarRetryAte(path string, t time.Time) error {
	return gravarAtomico(path, []byte(t.UTC().Format(time.RFC3339)+"\n"), 0600)
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return time.Minute
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

func tokenArquivoSemSegredo(path string, chaves ...string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var v interface{}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return acharToken(v, chaves...)
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
	return requestJSONMethod(ctx, client, http.MethodGet, url, nil, token, headers)
}

func requestJSONMethod(ctx context.Context, client *http.Client, method, rawURL string, body io.Reader, token string, headers map[string]string) (map[string]interface{}, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
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
		return nil, res.Header, &httpStatusError{status: res.StatusCode, header: res.Header}
	}
	var v map[string]interface{}
	if json.Unmarshal(b, &v) != nil {
		return nil, res.Header, errors.New("resposta inválida")
	}
	return v, res.Header, nil
}
func consultaClaude(ctx context.Context, base Instancia, client *http.Client) (Instancia, error) {
	dir := dirClaude(base)
	credPath := filepath.Join(dir, ".credentials.json")
	token, refresh, err := tokenClaudeValido(ctx, client, credPath)
	if err != nil || token == "" {
		return base, errors.New("credencial não encontrada")
	}
	url := os.Getenv("OPENHEINERSS_CLAUDE_USAGE_URL")
	if url == "" {
		url = "https://api.anthropic.com/api/oauth/usage"
	}
	v, headers, err := requestJSON(ctx, client, url, token, map[string]string{"anthropic-beta": "oauth-2025-04-20"})
	if he, ok := err.(*httpStatusError); ok && he.status == http.StatusUnauthorized && refresh != "" {
		if novo, refreshErr := renovarClaude(ctx, client, credPath, refresh); refreshErr == nil {
			v, headers, err = requestJSON(ctx, client, url, novo, map[string]string{"anthropic-beta": "oauth-2025-04-20"})
		} else {
			return base, fmt.Errorf("renovação OAuth falhou; mantendo fonte passiva")
		}
	}
	if err != nil {
		return base, err
	}
	i := instanciaUso(base, v, headers, "consulta-ativa")
	return i, nil
}

// reservaClaude usa uma única mensagem mínima para obter os cabeçalhos de cota
// quando /api/oauth/usage está temporariamente limitado. O cache/lock do
// chamador garante no máximo uma reserva por intervalo compartilhado por conta.
func reservaClaude(ctx context.Context, base Instancia, client *http.Client) (Instancia, error) {
	dir := dirClaude(base)
	credPath := filepath.Join(dir, ".credentials.json")
	token, refresh, err := tokenClaudeValido(ctx, client, credPath)
	if err != nil || token == "" {
		return base, errors.New("credencial não encontrada")
	}
	endpoint := os.Getenv("OPENHEINERSS_CLAUDE_MESSAGES_URL")
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	modelo := os.Getenv("OPENHEINERSS_CLAUDE_RESERVA_MODELO")
	if modelo == "" {
		modelo = "claude-haiku-4-5-20251001"
	}
	body, err := json.Marshal(map[string]interface{}{
		"model": modelo, "max_tokens": 1,
		"messages": []map[string]string{{"role": "user", "content": "ok"}},
	})
	if err != nil {
		return base, errors.New("reserva Claude inválida")
	}
	envia := func(tk string) (map[string]interface{}, http.Header, error) {
		return requestJSONMethod(ctx, client, http.MethodPost, endpoint, strings.NewReader(string(body)), tk, map[string]string{
			"Content-Type":      "application/json",
			"anthropic-version": "2023-06-01",
			"anthropic-beta":    "oauth-2025-04-20",
		})
	}
	v, headers, requestErr := envia(token)
	if he, ok := requestErr.(*httpStatusError); ok && he.status == http.StatusUnauthorized && refresh != "" {
		novo, refreshErr := renovarClaude(ctx, client, credPath, refresh)
		if refreshErr != nil {
			return base, errors.New("reserva: HTTP 401 e renovação OAuth falhou")
		}
		v, headers, requestErr = envia(novo)
	}
	if requestErr != nil && len(headers) == 0 {
		return base, requestErr
	}
	i := instanciaUso(base, v, headers, "cabeçalhos")
	if len(i.Janelas) == 0 {
		// Só NOMES de cabeçalhos entram no diagnóstico, nunca valores.
		status := "HTTP 2xx"
		if requestErr != nil {
			status = requestErr.Error()
		}
		return base, fmt.Errorf("reserva sem cabeçalhos de limite (%s; cabeçalhos anthropic-*: %s)", status, nomesCabecalhosAnthropic(headers))
	}
	return i, nil
}

// nomesCabecalhosAnthropic lista apenas os nomes (sem valores) dos cabeçalhos anthropic-*.
func nomesCabecalhosAnthropic(h http.Header) string {
	var nomes []string
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "anthropic-") {
			nomes = append(nomes, strings.ToLower(k))
		}
	}
	sort.Strings(nomes)
	if len(nomes) == 0 {
		return "nenhum"
	}
	return strings.Join(nomes, ",")
}

// tokenClaudeValido devolve o access token e, se ele já expirou (ou expira em
// menos de 1 min), renova antes de qualquer chamada: token vencido faz o
// servidor responder erro em vez de números e nunca chegava à renovação.
func tokenClaudeValido(ctx context.Context, client *http.Client, path string) (access, refresh string, err error) {
	access, refresh, err = credenciaisClaude(path)
	if err != nil || refresh == "" {
		return access, refresh, err
	}
	if exp := expiraEmClaude(path); !exp.IsZero() && time.Now().Add(time.Minute).After(exp) {
		novo, rerr := renovarClaude(ctx, client, path, refresh)
		if rerr != nil {
			return "", refresh, errors.New("token expirado e renovação OAuth falhou")
		}
		_, refresh2, _ := credenciaisClaude(path)
		if refresh2 != "" {
			refresh = refresh2
		}
		return novo, refresh, nil
	}
	return access, refresh, nil
}

func expiraEmClaude(path string) time.Time {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var root map[string]interface{}
	if json.Unmarshal(b, &root) != nil {
		return time.Time{}
	}
	oauth, _ := root["claudeAiOauth"].(map[string]interface{})
	ms, ok := num(oauth["expiresAt"])
	if !ok || ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

func consultaCodex(ctx context.Context, base Instancia, client *http.Client) (Instancia, error) {
	home := dirCodex(base)
	authPath := filepath.Join(home, "auth.json")
	token, err := tokenArquivo(authPath, "access_token", "accessToken", "token")
	if err != nil || token == "" {
		return base, errors.New("credencial não encontrada")
	}
	url := os.Getenv("OPENHEINERSS_CODEX_USAGE_URL")
	if url == "" {
		url = "https://chatgpt.com/backend-api/wham/usage"
	}
	reqHeaders := map[string]string{}
	if accountID := tokenArquivoSemSegredo(authPath, "account_id", "accountId"); accountID != "" {
		reqHeaders["ChatGPT-Account-Id"] = accountID
	}
	v, headers, err := requestJSON(ctx, client, url, token, reqHeaders)
	if err != nil {
		return base, err
	}
	return instanciaUso(base, v, headers, "consulta-ativa"), nil
}
func instanciaUso(base Instancia, v map[string]interface{}, h http.Header, fonte string) Instancia {
	i := Instancia{Nome: base.Nome, Base: base.Base, DadoEm: time.Now().Format(time.RFC3339), Fonte: fonte, ContaID: base.ContaID, ContaIDFonte: base.ContaIDFonte}
	if valor := headerValor(h, "anthropic-ratelimit-unified-5h-utilization", "anthropic-ratelimit-5h-utilization"); valor != "" {
		i.Fonte = "cabeçalhos"
		i.Janelas = append(i.Janelas, janelaCabecalho("5h", valor, headerValor(h, "anthropic-ratelimit-unified-5h-reset", "anthropic-ratelimit-5h-reset")))
	}
	if valor := headerValor(h, "anthropic-ratelimit-unified-7d-utilization", "anthropic-ratelimit-7d-utilization"); valor != "" {
		i.Fonte = "cabeçalhos"
		i.Janelas = append(i.Janelas, janelaCabecalho("semana", valor, headerValor(h, "anthropic-ratelimit-unified-7d-reset", "anthropic-ratelimit-7d-reset")))
	}
	walkUso(v, &i)
	return i
}

func headerValor(h http.Header, nomes ...string) string {
	for _, nome := range nomes {
		if valor := h.Get(nome); valor != "" {
			return valor
		}
	}
	return ""
}

func janelaCabecalho(nome, utilizacao, reset string) Janela {
	percentual := parseFloat(utilizacao)
	if percentual <= 1 {
		percentual *= 100
	}
	j := Janela{Nome: nome, Percentual: percentual}
	if valor := parseResetHeader(reset); valor != "" {
		j = comVoltaEm(j, valor)
	}
	return j
}

func parseResetHeader(valor string) string {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return ""
	}
	if f, err := strconv.ParseFloat(valor, 64); err == nil {
		if f > 1e12 {
			f /= 1000
		}
		return time.Unix(int64(f), 0).Format(time.RFC3339)
	}
	if t, err := http.ParseTime(valor); err == nil {
		return t.Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, valor); err == nil {
		return t.Format(time.RFC3339)
	}
	return ""
}
func walkUso(v interface{}, i *Instancia) {
	if m, ok := v.(map[string]interface{}); ok {
		for key, nome := range map[string]string{"five_hour": "5h", "seven_day": "semana", "five-hour": "5h", "seven-day": "semana", "primary_window": "5h", "secondary_window": "semana", "primary": "5h", "secondary": "semana", "primaryWindow": "5h", "secondaryWindow": "semana"} {
			if child, ok := m[key].(map[string]interface{}); ok {
				if percentual, found := percentualUso(child); found {
					j := Janela{Nome: nome, Percentual: percentual}
					if reset, ok := resetUso(child); ok {
						j = comVoltaEm(j, reset)
					}
					i.Janelas = append(i.Janelas, j)
				}
			}
		}
		for _, x := range m {
			walkUso(x, i)
		}
	}
	if a, ok := v.([]interface{}); ok {
		for _, x := range a {
			walkUso(x, i)
		}
	}
}

func percentualUso(m map[string]interface{}) (float64, bool) {
	for _, key := range []string{"used_percent", "used_percentage", "usedPercent", "usedPercentage", "utilization"} {
		if f, ok := num(m[key]); ok {
			if f <= 1 {
				f *= 100
			}
			return f, true
		}
	}
	return 0, false
}

func resetUso(m map[string]interface{}) (string, bool) {
	// A resposta WHAM também traz reset_after_seconds. Ele é relativo ao
	// instante da consulta e continua correto mesmo quando um fixture/cache foi
	// capturado alguns minutos antes; prefira-o ao timestamp absoluto antigo.
	if after, ok := num(m["reset_after_seconds"]); ok && after >= 0 {
		return time.Now().Add(time.Duration(after) * time.Second).Format(time.RFC3339), true
	}
	f, ok := num(m["resets_at"])
	if !ok {
		if s, ok := m["resets_at"].(string); ok {
			if valor := parseResetHeader(s); valor != "" {
				return valor, true
			}
		}
	}
	if !ok {
		f, ok = num(m["reset_at"])
	}
	if !ok {
		f, ok = num(m["resetAt"])
	}
	if !ok || f <= 0 {
		return "", false
	}
	if f > 1e12 {
		f /= 1000
	}
	return time.Unix(int64(f), 0).Format(time.RFC3339), true
}

func credenciaisClaude(path string) (access, refresh string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(b, &root); err != nil {
		return "", "", errors.New("credencial inválida")
	}
	oauth, ok := root["claudeAiOauth"].(map[string]interface{})
	if !ok {
		return "", "", errors.New("credencial OAuth não encontrada")
	}
	access, _ = oauth["accessToken"].(string)
	refresh, _ = oauth["refreshToken"].(string)
	return access, refresh, nil
}

func renovarClaude(ctx context.Context, client *http.Client, path, refresh string) (string, error) {
	tokenURL := os.Getenv("OPENHEINERSS_CLAUDE_OAUTH_TOKEN_URL")
	if tokenURL == "" {
		tokenURL = "https://platform.claude.com/v1/oauth/token"
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {claudeOAuthClientID}}
	v, _, err := requestJSONMethod(ctx, client, http.MethodPost, tokenURL, strings.NewReader(form.Encode()), "", map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return "", errors.New("renovação OAuth recusada")
	}
	access, ok := v["access_token"].(string)
	if !ok || access == "" {
		return "", errors.New("renovação OAuth recusada")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("credencial não encontrada")
	}
	var root map[string]interface{}
	if json.Unmarshal(b, &root) != nil {
		return "", errors.New("credencial inválida")
	}
	oauth, ok := root["claudeAiOauth"].(map[string]interface{})
	if !ok {
		return "", errors.New("credencial OAuth não encontrada")
	}
	oauth["accessToken"] = access
	if next, ok := v["refresh_token"].(string); ok && next != "" {
		oauth["refreshToken"] = next
	}
	if expires, ok := num(v["expires_in"]); ok {
		oauth["expiresAt"] = time.Now().Add(time.Duration(expires) * time.Second).UnixMilli()
	}
	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", errors.New("credencial inválida")
	}
	if err := gravarAtomico(path, updated, 0600); err != nil {
		return "", errors.New("credencial não pôde ser gravada")
	}
	return access, nil
}

func gravarAtomico(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func num(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case json.Number:
		f, e := x.Float64()
		return f, e == nil
	case string:
		f, e := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, e == nil
	}
	return 0, false
}
func parseFloat(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }
func dirClaude(i Instancia) string {
	if spec, ok := harness.CustomSpecFor(i.Nome); ok && spec.Env["CLAUDE_CONFIG_DIR"] != "" {
		return expandHome(spec.Env["CLAUDE_CONFIG_DIR"], filepath.Join(userHome(), ".claude"))
	}
	if strings.HasPrefix(i.Nome, "claude-") && i.Nome != "claude-code" {
		return filepath.Join(userHome(), "."+i.Nome)
	}
	return filepath.Join(userHome(), ".claude")
}
func dirCodex(i Instancia) string {
	if spec, ok := harness.CustomSpecFor(i.Nome); ok && spec.Env["CODEX_HOME"] != "" {
		return expandHome(spec.Env["CODEX_HOME"], filepath.Join(userHome(), ".codex"))
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
