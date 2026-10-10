package opencode

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCodeExternalDirectoryMesclaConfig(t *testing.T) {
	env := []string{"OPENCODE_CONFIG_CONTENT={\"mcp\":{\"x\":{}}}"}
	b, err := opencodeExternalDirectoryConfig(env, "/tmp/wt/a", []string{"/fora", "/outra"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["mcp"] == nil {
		t.Fatal("configuração anterior foi perdida")
	}
	p := got["permission"].(map[string]interface{})["external_directory"].(map[string]interface{})
	if p["/fora/**"] != "allow" || p["/outra/**"] != "allow" {
		t.Fatalf("permissões=%v", p)
	}
}

func TestOpenCodeLeituraSistemaNegaEscritaFora(t *testing.T) {
	b, err := opencodeExternalDirectoryConfig(nil, "/tmp/wt/a", []string{"/tmp/wt/liberada"}, []string{"/proc", "/etc"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"/proc/**":"allow"`, `"/etc/**":"allow"`, `"/tmp/wt/liberada/**":"allow"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("falta %s em %s", want, s)
		}
	}
	// a deny de "fora da worktree" precisa vir antes da allow da pasta permitida (vale a última regra)
	deny := strings.Index(s, `"../**":"deny"`)
	allow := strings.Index(s, `"../liberada/**":"allow"`)
	if deny < 0 || allow < 0 || deny > allow {
		t.Fatalf("ordem das regras de edit errada: %s", s)
	}
	if strings.Contains(s, `"/proc/**":"deny"`) || strings.Contains(s, `"edit":{"*":"allow","/`) {
		t.Fatalf("regra de edit inesperada: %s", s)
	}
}
