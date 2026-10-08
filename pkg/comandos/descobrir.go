package comandos

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom-org/openheinerss/pkg/harness"
)

// Descoberta em tempo de execução: os comandos e skills que o usuário instalou no harness ficam
// em arquivos .md conhecidos. Ler esses arquivos é barato e não chama o binário do harness.
// (aider e agy não expõem a lista fora da tela: ficam só com o catálogo embutido.)

// envDe devolve o env da instância (ex.: CLAUDE_CONFIG_DIR da conta2), que vence o do processo.
func envDe(nome string) map[string]string {
	if spec, ok := harness.CustomSpecFor(nome); ok {
		return spec.Env
	}
	return nil
}

func getenv(env map[string]string, k string) string {
	if v, ok := env[k]; ok {
		return expandirHome(v)
	}
	return expandirHome(os.Getenv(k))
}

func expandirHome(v string) string {
	if v == "~" || strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(v, "~"))
		}
	}
	return v
}

func descobrir(base string, env map[string]string, cwd string) []Comando {
	home, _ := os.UserHomeDir()
	var out []Comando
	switch base {
	case "claude-code":
		cfg := getenv(env, "CLAUDE_CONFIG_DIR")
		if cfg == "" && home != "" {
			cfg = filepath.Join(home, ".claude")
		}
		var raizes []string
		if cwd != "" {
			raizes = append(raizes, filepath.Join(cwd, ".claude"))
		}
		if cfg != "" {
			raizes = append(raizes, cfg)
		}
		for _, r := range raizes {
			out = append(out, comandosMD(filepath.Join(r, "commands"), "", RepasseLiteral, "comando personalizado do Claude Code; vai literal ao claude -p")...)
			out = append(out, skills(filepath.Join(r, "skills"), RepasseLiteral, "skill do Claude Code; vai literal ao claude -p")...)
		}
	case "opencode":
		var raizes []string
		if cwd != "" {
			raizes = append(raizes, filepath.Join(cwd, ".opencode"))
		}
		xdg := getenv(env, "XDG_CONFIG_HOME")
		if xdg == "" && home != "" {
			xdg = filepath.Join(home, ".config")
		}
		if xdg != "" {
			raizes = append(raizes, filepath.Join(xdg, "opencode"))
		}
		for _, r := range raizes {
			for _, d := range []string{"command", "commands"} {
				out = append(out, comandosMD(filepath.Join(r, d), "", RepasseTraduzido, detOpencodeCmd)...)
			}
		}
	case "codex":
		dir := getenv(env, "CODEX_HOME")
		if dir == "" && home != "" {
			dir = filepath.Join(home, ".codex")
		}
		if dir != "" {
			out = append(out, comandosMD(filepath.Join(dir, "prompts"), "prompts:", RepasseSemEquivalente, "prompt personalizado do codex interativo; o codex exec não expande")...)
		}
	}
	return out
}

// comandosMD lê <dir>/**/*.md: sub/nome.md vira /sub:nome; a descrição vem do frontmatter.
func comandosMD(dir, prefixo, repasse, detalhe string) []Comando {
	var out []Comando
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		nome := strings.ReplaceAll(strings.TrimSuffix(rel, ".md"), string(filepath.Separator), ":")
		n, err := NormalizarNome(prefixo + nome)
		if err != nil {
			return nil
		}
		out = append(out, Comando{Nome: n, Descricao: descricaoMD(p), Repasse: repasse, Detalhe: detalhe, Origem: OrigemDescoberto})
		return nil
	})
	return out
}

// skills lê <dir>/<nome>/SKILL.md.
func skills(dir, repasse, detalhe string) []Comando {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Comando
	for _, e := range entradas {
		p := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		n, err := NormalizarNome(e.Name())
		if err != nil {
			continue
		}
		out = append(out, Comando{Nome: n, Descricao: descricaoMD(p), Repasse: repasse, Detalhe: detalhe, Origem: OrigemDescoberto})
	}
	return out
}

// descricaoMD pega "description:" do frontmatter; sem ele, a primeira linha de texto.
func descricaoMD(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	emFrontmatter, primeira := false, ""
	for i := 0; s.Scan() && i < 60; i++ {
		l := strings.TrimSpace(s.Text())
		if i == 0 && l == "---" {
			emFrontmatter = true
			continue
		}
		if emFrontmatter {
			if l == "---" {
				emFrontmatter = false
				continue
			}
			if v, ok := strings.CutPrefix(l, "description:"); ok {
				return curto(strings.Trim(strings.TrimSpace(v), `"'`))
			}
			continue
		}
		if primeira == "" && l != "" {
			primeira = strings.TrimLeft(l, "# ")
		}
	}
	return curto(primeira)
}

func curto(s string) string {
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}
