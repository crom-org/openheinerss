package harness

import "testing"

func TestSelecaoMCP(t *testing.T) {
	t.Setenv("OPENHEINERSS_MCP", "a,b")
	casos := []struct {
		opts      map[string]interface{}
		env       map[string]string
		desligado bool
		nomes     int
	}{
		{nil, nil, false, 2},
		{nil, map[string]string{"OPENHEINERSS_MCP": "nenhum"}, true, 0},
		{map[string]interface{}{OptionSemMCP: true}, nil, true, 0},
		{map[string]interface{}{OptionMCP: false}, nil, true, 0},
		{map[string]interface{}{OptionMCP: true}, nil, false, 0},
		{map[string]interface{}{OptionMCP: "x"}, nil, false, 1},
		{map[string]interface{}{OptionMCP: []interface{}{"x", "y", "z"}}, nil, false, 3},
		{map[string]interface{}{OptionMCP: []interface{}{}}, nil, true, 0},
	}
	for i, k := range casos {
		sel := SelecaoMCP(SessionConfig{Options: k.opts, Env: k.env})
		if sel.Desligado != k.desligado || len(sel.Nomes) != k.nomes {
			t.Errorf("caso %d: %+v", i, sel)
		}
	}
}
