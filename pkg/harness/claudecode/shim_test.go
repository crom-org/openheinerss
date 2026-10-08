package claudecode

import (
	"strings"
	"testing"
)

func TestWorkerTraduzModoAskDoProtocolo(t *testing.T) {
	if !strings.Contains(NodeWorkerScript, "permissionMode === 'ask'") {
		t.Fatal("worker do Claude deve traduzir permissionMode=ask para manual")
	}
}
