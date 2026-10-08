package orchestrator

// Checkpoint git-sombra por turno. O `rodar` (não o motor) grava, na worktree do agente, um commit
// "sombra" da árvore de trabalho inteira (arquivos rastreados e novos, respeitando o .gitignore) em
// refs/openheinerss/<nome>/<n>. Nada disso toca o índice real, o HEAD ou o histórico da branch do agente:
// a árvore é montada num índice temporário (GIT_INDEX_FILE) e o commit só aponta para o HEAD como pai.
// Mesmo estado de árvore que o último checkpoint não gera outro.
//
// Fora de um repositório git nada é feito aqui: quem precisa de checkpoint sem git usa o pkg/checkpoint
// (cópia de arquivos), que segue como alternativa do servidor e dos SDKs.
//
// Desfazer (DesfazerAgente) restaura a árvore SÓ na worktree do agente. Escolha para o HEAD/branch: se o
// checkpoint foi gravado com outro HEAD, a branch do agente volta ao pai registrado com `git reset --soft`
// (os commits de depois continuam alcançáveis pelo próprio checkpoint "antes-de-desfazer", que tem o HEAD
// antigo como pai; nada se perde). Depois `git clean -fd` (sem -x: ignorados ficam) e
// `git read-tree -u --reset <árvore>` deixam índice e arquivos iguais ao checkpoint.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MotivoAntesDeDesfazer é o motivo do checkpoint criado antes de uma restauração.
const MotivoAntesDeDesfazer = "antes-de-desfazer"

// Checkpoint é um ponto de restauração git-sombra.
type Checkpoint struct {
	N      int    `json:"n"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	Em     string `json:"em"`
	Motivo string `json:"motivo,omitempty"`
}

// CheckpointInfo é um Checkpoint com o resumo do que mudou desde o anterior.
type CheckpointInfo struct {
	Checkpoint
	Resumo string `json:"resumo,omitempty"`
}

func refCheckpoints(nome string) string { return "refs/openheinerss/" + nome + "/" }

func gitSaida(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// ehRepositorioGit diz se dir está dentro de um repositório git.
func ehRepositorioGit(dir string) bool {
	_, err := gitSaida(dir, nil, "rev-parse", "--git-dir")
	return err == nil
}

// headDe devolve o sha do HEAD de dir ("" se não for repositório ou não houver commit).
func headDe(dir string) string {
	out, err := gitSaida(dir, nil, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return ""
	}
	return out
}

// ListarCheckpoints devolve os checkpoints de nome, do mais antigo ao mais novo.
func ListarCheckpoints(dir, nome string) ([]Checkpoint, error) {
	out, err := gitSaida(dir, nil, "for-each-ref", "--format=%(refname)%09%(objectname)%09%(committerdate:iso-strict)%09%(contents:subject)", refCheckpoints(nome))
	if err != nil {
		return nil, err
	}
	var lista []Checkpoint
	for _, linha := range strings.Split(out, "\n") {
		partes := strings.SplitN(linha, "\t", 4)
		if len(partes) < 4 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(partes[0], refCheckpoints(nome)))
		if err != nil {
			continue
		}
		lista = append(lista, Checkpoint{N: n, Ref: partes[0], SHA: partes[1], Em: partes[2], Motivo: partes[3]})
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].N < lista[j].N })
	return lista, nil
}

// arvoreSombra monta a árvore do estado atual da worktree sem tocar o índice real.
func arvoreSombra(dir string) (string, error) {
	tmp, err := os.MkdirTemp("", "oh-indice-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitSaida(dir, env, "add", "-A"); err != nil {
		return "", err
	}
	return gitSaida(dir, env, "write-tree")
}

// CheckpointGit grava um checkpoint da worktree dir para o agente nome. Devolve ok=false (sem erro) quando
// dir não é repositório git ou quando a árvore não mudou desde o último checkpoint.
func CheckpointGit(dir, nome, motivo string) (cp Checkpoint, ok bool, err error) {
	if dir == "" || !ehRepositorioGit(dir) {
		return Checkpoint{}, false, nil
	}
	arvore, err := arvoreSombra(dir)
	if err != nil {
		return Checkpoint{}, false, err
	}
	existentes, err := ListarCheckpoints(dir, nome)
	if err != nil {
		return Checkpoint{}, false, err
	}
	n := 1
	if len(existentes) > 0 {
		ultimo := existentes[len(existentes)-1]
		if t, err := gitSaida(dir, nil, "rev-parse", ultimo.SHA+"^{tree}"); err == nil && t == arvore {
			return ultimo, false, nil
		}
		n = ultimo.N + 1
	}
	args := []string{"commit-tree", arvore, "-m", motivo}
	if head := headDe(dir); head != "" {
		args = append(args, "-p", head)
	}
	env := []string{"GIT_AUTHOR_NAME=openheinerss", "GIT_AUTHOR_EMAIL=openheinerss@localhost", "GIT_COMMITTER_NAME=openheinerss", "GIT_COMMITTER_EMAIL=openheinerss@localhost"}
	sha, err := gitSaida(dir, env, args...)
	if err != nil {
		return Checkpoint{}, false, err
	}
	ref := refCheckpoints(nome) + strconv.Itoa(n)
	if _, err := gitSaida(dir, nil, "update-ref", ref, sha); err != nil {
		return Checkpoint{}, false, err
	}
	return Checkpoint{N: n, Ref: ref, SHA: sha, Em: time.Now().Format(time.RFC3339), Motivo: motivo}, true, nil
}

// ApagarCheckpoints remove refs/openheinerss/<nome>/* (para a limpeza do agente e da sua worktree).
func ApagarCheckpoints(dir, nome string) error {
	lista, err := ListarCheckpoints(dir, nome)
	if err != nil {
		return err
	}
	for _, c := range lista {
		if _, err := gitSaida(dir, nil, "update-ref", "-d", c.Ref); err != nil {
			return err
		}
	}
	return nil
}

// ResumoCheckpoints lista os checkpoints com o `git diff --shortstat` contra o anterior (o primeiro, contra o seu HEAD).
func ResumoCheckpoints(dir, nome string) ([]CheckpointInfo, error) {
	lista, err := ListarCheckpoints(dir, nome)
	if err != nil {
		return nil, err
	}
	res := make([]CheckpointInfo, 0, len(lista))
	for i, c := range lista {
		base := c.SHA + "^"
		if i > 0 {
			base = lista[i-1].SHA
		}
		resumo, err := gitSaida(dir, nil, "diff", "--shortstat", base, c.SHA)
		if err != nil { // primeiro checkpoint sem pai (repositório sem commits)
			resumo = ""
		}
		if resumo == "" {
			resumo = "sem mudanças"
		}
		res = append(res, CheckpointInfo{Checkpoint: c, Resumo: resumo})
	}
	return res, nil
}

// registroTurnos liga o checkpoint git-sombra e os campos ultimo_evento_em/head/checkpoints ao meta do rodar.
type registroTurnos struct {
	work, nome string
	cur        *meta
	metaPath   string
	now        func() time.Time
	ativo      bool
	ultimaEsc  time.Time
}

func novoRegistroTurnos(work, nome string, cur *meta, metaPath string, now func() time.Time) *registroTurnos {
	// Missões são somente leitura e rodam numa pasta descartável: sem checkpoint.
	ativo := work != "" && !strings.HasPrefix(nome, "missao-") && ehRepositorioGit(work)
	return &registroTurnos{work: work, nome: nome, cur: cur, metaPath: metaPath, now: now, ativo: ativo}
}

// TocarEvento atualiza ultimo_evento_em no meta, no máximo uma escrita a cada 5 s.
func (r *registroTurnos) TocarEvento() {
	agora := r.now()
	if !r.ultimaEsc.IsZero() && agora.Sub(r.ultimaEsc) < 5*time.Second {
		return
	}
	r.ultimaEsc = agora
	r.cur.UltimoEventoEm = agora.Format(time.RFC3339)
	_ = writeMeta(r.metaPath, *r.cur)
}

// Marcar grava o checkpoint do fim de um turno/tentativa (ou da base) e atualiza head e checkpoints no meta.
func (r *registroTurnos) Marcar(motivo string) {
	if !r.ativo {
		return
	}
	cp, ok, err := CheckpointGit(r.work, r.nome, motivo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "AVISO: checkpoint git não gravado: "+err.Error())
	}
	if ok {
		r.cur.Checkpoints = append(r.cur.Checkpoints, cp)
	}
	r.cur.Head = headDe(r.work)
	_ = writeMeta(r.metaPath, *r.cur)
}

// DesfazerAgente restaura a worktree do agente para o checkpoint n (n <= 0: o anterior ao último).
// Devolve o checkpoint restaurado. Recusa se o agente está rodando, a menos que forcar.
func DesfazerAgente(agentsDir, nome string, n int, forcar bool) (Checkpoint, error) {
	logs := filepath.Join(agentsDir, "logs")
	dir := filepath.Join(agentsDir, nome)
	var m meta
	temMeta := false
	if b, err := os.ReadFile(filepath.Join(logs, nome+".meta.json")); err == nil {
		temMeta = json.Unmarshal(b, &m) == nil
		if m.Worktree != "" {
			dir = m.Worktree
		}
		if m.Fim == "" && estaVivo(m) && !forcar {
			return Checkpoint{}, fmt.Errorf("agente %s está rodando (PID %d); pare-o ou use --forcar", nome, m.PID)
		}
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return Checkpoint{}, fmt.Errorf("worktree do agente %s não encontrada em %s", nome, dir)
	}
	// Nunca no repositório principal: só numa worktree ligada (git-dir diferente do git-common-dir).
	gd, e1 := gitSaida(dir, nil, "rev-parse", "--path-format=absolute", "--git-dir")
	cd, e2 := gitSaida(dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e1 != nil || e2 != nil {
		return Checkpoint{}, fmt.Errorf("%s não é um repositório git", dir)
	}
	if gd == cd {
		return Checkpoint{}, fmt.Errorf("%s é o repositório principal, não a worktree de um agente: não restauro", dir)
	}
	lista, err := ListarCheckpoints(dir, nome)
	if err != nil {
		return Checkpoint{}, err
	}
	var alvo *Checkpoint
	if n > 0 {
		for i := range lista {
			if lista[i].N == n {
				alvo = &lista[i]
			}
		}
		if alvo == nil {
			return Checkpoint{}, fmt.Errorf("agente %s não tem o checkpoint %d", nome, n)
		}
	} else {
		if len(lista) < 2 {
			return Checkpoint{}, fmt.Errorf("agente %s tem %d checkpoint(s); preciso de ao menos 2 para voltar ao anterior (ou informe n)", nome, len(lista))
		}
		alvo = &lista[len(lista)-2]
	}
	arvore, err := gitSaida(dir, nil, "rev-parse", alvo.SHA+"^{tree}")
	if err != nil {
		return Checkpoint{}, err
	}
	antes, gravado, err := CheckpointGit(dir, nome, MotivoAntesDeDesfazer)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("não consegui guardar o estado atual antes de desfazer: %w", err)
	}
	if gravado && temMeta {
		m.Checkpoints = append(m.Checkpoints, antes)
		_ = writeMeta(filepath.Join(logs, nome+".meta.json"), m)
	}
	if pai, err := gitSaida(dir, nil, "rev-parse", "--verify", "--quiet", alvo.SHA+"^"); err == nil && pai != headDe(dir) {
		if _, err := gitSaida(dir, nil, "reset", "--soft", pai); err != nil {
			return Checkpoint{}, err
		}
	}
	if _, err := gitSaida(dir, nil, "clean", "-fd"); err != nil {
		return Checkpoint{}, err
	}
	if _, err := gitSaida(dir, nil, "read-tree", "-u", "--reset", arvore); err != nil {
		return Checkpoint{}, err
	}
	return *alvo, nil
}
