package versoes

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EmUso lista os processos que estão usando o harness: o executável dele (o do harness em si,
// ou o script que um interpretador roda) e os `rodar` cuja instância/harness é da base.
// Cada processo é classificado pela cadeia de pais: filho de um `rodar` é "rodar", de um
// `serve` é "serve"; o resto (por exemplo uma sessão sua no terminal) é "processo".
func (e Ambiente) EmUso(base, caminho string) []Uso {
	if e.ProcRoot == "" {
		return nil
	}
	ents, err := os.ReadDir(e.ProcRoot)
	if err != nil {
		return nil
	}
	type proc struct {
		pid, ppid int
		argv      []string
		usa       bool
	}
	procs := map[int]*proc{}
	alvo := resolver(caminho)
	for _, en := range ents {
		pid, err := strconv.Atoi(en.Name())
		if err != nil || pid == e.PID {
			continue
		}
		dir := filepath.Join(e.ProcRoot, en.Name())
		cl, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil || len(cl) == 0 {
			continue
		}
		p := &proc{pid: pid, argv: strings.Split(string(bytes.TrimRight(cl, "\x00")), "\x00")}
		p.ppid = ppidDe(filepath.Join(dir, "stat"))
		cand := []string{}
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			cand = append(cand, strings.TrimSuffix(exe, " (deleted)"))
		}
		for i := 0; i < len(p.argv) && i < 2; i++ {
			if filepath.IsAbs(p.argv[i]) {
				cand = append(cand, p.argv[i])
			}
		}
		for _, c := range cand {
			if alvo != "" && resolver(c) == alvo {
				p.usa = true
			}
		}
		if !p.usa && e.BaseDe != nil && tipoPorArgv(p.argv) == "rodar" {
			for _, a := range p.argv[1:] {
				if !strings.HasPrefix(a, "-") && e.BaseDe(a) == base {
					p.usa = true
				}
			}
		}
		procs[pid] = p
	}
	var out []Uso
	for pid, p := range procs {
		if !p.usa {
			continue
		}
		tipo := "processo"
		for cur, n := p, 0; cur != nil && n < 32; n++ {
			if t := tipoPorArgv(cur.argv); t != "" {
				tipo = t
				break
			}
			cur = procs[cur.ppid]
		}
		out = append(out, Uso{PID: pid, Tipo: tipo, Comando: resumirComando(p.argv)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

func resolver(p string) string {
	if p == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// tipoPorArgv reconhece um processo do próprio openheinerss (`rodar` ou `serve`).
func tipoPorArgv(argv []string) string {
	if len(argv) == 0 || !strings.HasPrefix(filepath.Base(argv[0]), "openheinerss") {
		return ""
	}
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if a == "--projeto" || a == "--config" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		switch a {
		case "serve", "servir":
			return "serve"
		case "rodar":
			return "rodar"
		}
		return ""
	}
	return ""
}

// ppidDe lê o PPID de /proc/<pid>/stat (o nome do processo pode ter espaços e parênteses).
func ppidDe(statPath string) int {
	b, err := os.ReadFile(statPath)
	if err != nil {
		return 0
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

// resumirComando junta o argv em uma linha curta (prompts enormes não entram no relatório).
func resumirComando(argv []string) string {
	linha := strings.Join(strings.Fields(strings.Join(argv, " ")), " ")
	if r := []rune(linha); len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return linha
}
