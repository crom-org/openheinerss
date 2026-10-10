package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// regrasOrdenadas serializa as regras de permissão do opencode na ordem de inserção: ele
// avalia a última regra que casa, então a ordem faz parte do significado.
type regrasOrdenadas struct {
	chaves []string
	acoes  map[string]string
}

func (r *regrasOrdenadas) set(k, acao string) {
	if r.acoes == nil {
		r.acoes = map[string]string{}
	}
	if _, ok := r.acoes[k]; !ok {
		r.chaves = append(r.chaves, k)
	}
	r.acoes[k] = acao
}

func (r regrasOrdenadas) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range r.chaves {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		ab, _ := json.Marshal(r.acoes[k])
		b.Write(kb)
		b.WriteByte(':')
		b.Write(ab)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func regrasExistentes(v interface{}) *regrasOrdenadas {
	r := &regrasOrdenadas{}
	switch t := v.(type) {
	case string:
		r.set("*", t)
	case map[string]interface{}:
		ks := make([]string, 0, len(t))
		for k := range t {
			ks = append(ks, k)
		}
		sortStrings(ks)
		for _, k := range ks {
			if s, ok := t[k].(string); ok {
				r.set(k, s)
			}
		}
	}
	return r
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// AplicarPermissoesPastasOpenCode escreve em root["permission"] as regras de pasta do opencode:
//   - external_directory: libera leitura e escrita nas pastas permitidas e só leitura nas de leitura
//     (o opencode não separa leitura de escrita nesse portão);
//   - edit: nega toda escrita fora da worktree (padrão relativo "../**", que é como o opencode
//     casa caminhos de edição) e reabre só as pastas permitidas. A deny vem antes das allows.
//
// bash não passa pelo `edit`: um comando de shell escrevendo fora continua sujeito só às permissões do SO.
func AplicarPermissoesPastasOpenCode(root map[string]interface{}, cwd string, escrita, leitura []string) {
	perm, _ := root["permission"].(map[string]interface{})
	if perm == nil {
		perm = map[string]interface{}{}
		root["permission"] = perm
	}
	ext := regrasExistentes(perm["external_directory"])
	for _, d := range append(append([]string(nil), leitura...), escrita...) {
		d = strings.TrimRight(d, "/")
		if d == "" {
			continue
		}
		ext.set(d, "allow")
		ext.set(d+"/**", "allow")
	}
	perm["external_directory"] = ext
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd == "" {
		return
	}
	cwd = filepath.Clean(cwd)
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = real
	}
	edit := regrasExistentes(perm["edit"])
	if len(edit.chaves) == 0 {
		edit.set("*", "allow")
	}
	edit.set("../**", "deny")
	for _, d := range escrita {
		rel, err := filepath.Rel(cwd, filepath.Clean(d))
		if err != nil || rel == "." || !strings.HasPrefix(rel, "..") {
			continue
		}
		edit.set(filepath.ToSlash(rel), "allow")
		edit.set(filepath.ToSlash(rel)+"/**", "allow")
	}
	perm["edit"] = edit
}
