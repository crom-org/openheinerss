//go:build linux

package process

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Processo "runner" de mentira: inicia um filho pelo Configure e morre sem limpar nada.
func TestHelperRunnerQueMorre(t *testing.T) {
	if os.Getenv("OH_HELPER_RUNNER") != "1" {
		t.Skip("só como filho do teste")
	}
	cmd := exec.CommandContext(context.Background(), "sleep", "300")
	Configure(cmd)
	if err := cmd.Start(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.WriteString(strconv.Itoa(cmd.Process.Pid) + "\n")
	os.Exit(3) // sem Wait, sem Kill: o filho só morre se o kernel fizer isso
}

func TestFilhoMorreQuandoORunnerMorre(t *testing.T) {
	runner := exec.Command(os.Args[0], "-test.run=^TestHelperRunnerQueMorre$")
	runner.Env = append(os.Environ(), "OH_HELPER_RUNNER=1")
	out, err := runner.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	linha, _ := bufio.NewReader(out).ReadString('\n')
	_ = runner.Wait() // o runner saiu com 3
	pid, err := strconv.Atoi(strings.TrimSpace(linha))
	if err != nil {
		t.Fatalf("pid do filho: %q", linha)
	}
	// Limpeza caso o teste falhe: só o filho que este teste criou, com SIGTERM.
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	vivo := func() bool {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		return err == nil && !strings.Contains(string(b), ") Z ")
	}
	deadline := time.Now().Add(3 * time.Second)
	for vivo() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if vivo() {
		t.Fatalf("o filho %d continuou vivo depois que o runner morreu", pid)
	}
}
