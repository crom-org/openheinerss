package doctor

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/crom-org/openheinerss/pkg/protocol"
)

type toolCheck struct {
	name         string
	binary       string
	required     bool
	versionCmd   []string
	suggestedFix string
}

var toolsToCheck = []toolCheck{
	{
		name:         "Git",
		binary:       "git",
		required:     true,
		versionCmd:   []string{"git", "--version"},
		suggestedFix: "Instale o Git através do gerenciador de pacotes do seu sistema (ex: apt install git).",
	},
	{
		name:         "Go Runtime",
		binary:       "go",
		required:     false,
		versionCmd:   []string{"go", "version"},
		suggestedFix: "Instale o Go v1.22+ via https://go.dev/dl/ ou gerenciador de pacotes.",
	},
	{
		name:         "Node.js Runtime",
		binary:       "node",
		required:     false, // obrigatório para modo SDK de alguns harnesses
		versionCmd:   []string{"node", "-v"},
		suggestedFix: "Instale o Node.js v18+ via nvm ('nvm install 20') ou https://nodejs.org.",
	},
	{
		name:         "Claude Code CLI",
		binary:       "claude",
		required:     false,
		versionCmd:   []string{"claude", "--version"},
		suggestedFix: "Instale via 'npm install -g @anthropic-ai/claude-code'.",
	},
	{
		name:         "OpenCode CLI",
		binary:       "opencode",
		required:     false,
		versionCmd:   []string{"opencode", "--version"},
		suggestedFix: "Instale o OpenCode CLI ou adicione ao PATH do sistema.",
	},
	{
		name:         "Docker",
		binary:       "docker",
		required:     false,
		versionCmd:   []string{"docker", "--version"},
		suggestedFix: "Instale o Docker Engine / Desktop para containers de modelos locais.",
	},
}

// CheckEnvironment analisa as ferramentas instaladas no sistema operacional
func CheckEnvironment(targetHarness string) protocol.DoctorCheckResult {
	var items []protocol.DoctorItem
	allOk := true
	hasWarnings := false

	for _, t := range toolsToCheck {
		item := protocol.DoctorItem{
			Name:         t.name,
			Required:     t.required,
			SuggestedFix: t.suggestedFix,
		}

		path, err := exec.LookPath(t.binary)
		if err != nil {
			item.Installed = false
			if t.required {
				allOk = false
			} else {
				hasWarnings = true
			}
		} else {
			item.Installed = true
			item.Path = path
			if len(t.versionCmd) > 0 {
				out, vErr := exec.Command(t.versionCmd[0], t.versionCmd[1:]...).Output()
				if vErr == nil {
					item.Version = strings.TrimSpace(string(out))
				}
			}
		}
		items = append(items, item)
	}

	status := "ok"
	summary := "Todos os componentes essenciais estão operacionais."
	if !allOk {
		status = "error"
		summary = "Dependências essenciais não foram encontradas no sistema."
	} else if hasWarnings {
		status = "warning"
		summary = "O sistema base está pronto, mas alguns harnesses opcionais precisam de ferramentas adicionais."
	}

	return protocol.DoctorCheckResult{
		Status:  status,
		Items:   items,
		Summary: summary,
	}
}

// FormatDoctorReport formata o resultado para visualização no terminal CLI
func FormatDoctorReport(res protocol.DoctorCheckResult) string {
	var sb strings.Builder
	sb.WriteString("🩺 Openheinerss Doctor - Diagnóstico do Sistema\n\n")

	for _, item := range res.Items {
		icon := "✅"
		statusText := "Disponível"
		if !item.Installed {
			if item.Required {
				icon = "❌"
				statusText = "Ausente (Obrigatório)"
			} else {
				icon = "⚠️ "
				statusText = "Opcional não encontrado"
			}
		}

		sb.WriteString(fmt.Sprintf("%s %-20s : %s\n", icon, item.Name, statusText))
		if item.Installed {
			if item.Version != "" {
				sb.WriteString(fmt.Sprintf("   Versão: %s\n", item.Version))
			}
			if item.Path != "" {
				sb.WriteString(fmt.Sprintf("   Local:  %s\n", item.Path))
			}
		} else if item.SuggestedFix != "" {
			sb.WriteString(fmt.Sprintf("   Como resolver: %s\n", item.SuggestedFix))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Resumo: %s\n", res.Summary))
	return sb.String()
}
