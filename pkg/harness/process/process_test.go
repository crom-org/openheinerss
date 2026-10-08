//go:build linux

package process

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// O próprio binário de teste faz o papel do openheinerss: entende `__supervisor`.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == SupervisorArg {
		os.Exit(RunSupervisor(os.Args[2:]))
	}
	if os.Getenv("OH_SEM_SUPERVISOR") != "1" {
		UseSupervisor(os.Args[0])
	}
	os.Exit(m.Run())
}

// Processo "runner" de mentira: inicia um filho pelo Configure e morre sem limpar nada.
func TestHelperRunnerQueMorre(t *testing.T) {
	if os.Getenv("OH_HELPER_RUNNER") != "1" {
		t.Skip("só como filho do teste")
	}
	cmd := exec.CommandContext(context.Background(), "sleep", "300")
	if os.Getenv("OH_HELPER_NETO") == "1" {
		// motor falso que cria um neto `sleep 300` e imprime o pid dele
		cmd = exec.CommandContext(context.Background(), "sh", "-c", "sleep 300 & echo $!; wait")
	}
	Configure(cmd)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	if os.Getenv("OH_HELPER_NETO") == "1" {
		linha, _ := bufio.NewReader(out).ReadString('\n')
		os.Stdout.WriteString(linha)
		time.Sleep(time.Hour) // o teste mata este runner com SIGKILL
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

func vivoPid(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	return err == nil && !strings.Contains(string(b), ") Z ")
}

// Runner morto com SIGKILL e motor que cria um neto: neto e filho precisam morrer.
func TestNetoMorreQuandoORunnerMorreComSIGKILL(t *testing.T) {
	runner := exec.Command(os.Args[0], "-test.run=^TestHelperRunnerQueMorre$")
	runner.Env = append(os.Environ(), "OH_HELPER_RUNNER=1", "OH_HELPER_NETO=1")
	out, _ := runner.StdoutPipe()
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	linha, _ := bufio.NewReader(out).ReadString('\n')
	neto, err := strconv.Atoi(strings.TrimSpace(linha))
	if err != nil {
		_ = runner.Process.Kill()
		t.Fatalf("pid do neto: %q", linha)
	}
	t.Cleanup(func() { _ = syscall.Kill(neto, syscall.SIGTERM) })
	if !vivoPid(neto) {
		t.Fatal("neto deveria estar vivo antes do SIGKILL")
	}
	_ = runner.Process.Kill()
	_ = runner.Wait()
	deadline := time.Now().Add(6 * time.Second)
	for vivoPid(neto) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if vivoPid(neto) {
		t.Fatalf("neto %d sobreviveu à morte do runner", neto)
	}
}

func TestSupervisorPreservaStdinEStdout(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "cat")
	Configure(cmd)
	cmd.Stdin = strings.NewReader("MARCADOR-R7")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "MARCADOR-R7" {
		t.Fatalf("stdin perdido: %q", out.String())
	}
}

func TestSupervisorRepassaCodigoDeSaida(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 7")
	Configure(cmd)
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 7 {
		t.Fatalf("esperava código 7, veio %v", err)
	}
}

// Sem binário supervisor (uso como biblioteca) o comando continua rodando e o grupo morre no Cancel.
func TestSemSupervisorStdinFunciona(t *testing.T) {
	old := supervisorBin.Load()
	supervisorBin.Store("")
	defer func() { supervisorBin.Store(old) }()
	cmd := exec.CommandContext(context.Background(), "cat")
	Configure(cmd)
	cmd.Stdin = strings.NewReader("abc")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil || out.String() != "abc" {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
}
