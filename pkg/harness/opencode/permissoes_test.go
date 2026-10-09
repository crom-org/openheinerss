package opencode

import (
	"encoding/json"
	"testing"
)

func TestOpenCodeExternalDirectoryMesclaConfig(t *testing.T) {
	env := []string{"OPENCODE_CONFIG_CONTENT={\"mcp\":{\"x\":{}}}"}
	b, err := opencodeExternalDirectoryConfig(env, []string{"/fora", "/outra"})
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
