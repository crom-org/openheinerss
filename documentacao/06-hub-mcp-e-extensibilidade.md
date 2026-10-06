# 06 - Hub Centralizado de MCP (Model Context Protocol)

O **Model Context Protocol (MCP)** é o padrão aberto para conectar modelos de linguagem a ferramentas externas, bancos de dados, navegadores e APIs de desenvolvimento.

No ecossistema tradicional, cada ferramenta exige seu próprio arquivo de configuração duplicado. O Openheinerss resolve essa dor atuando como um **Hub Centralizado de MCP**.

---

## 1. O Arquivo Central `.openheinerss/mcp.json`

Ao rodar `openheinerss init`, o arquivo `.openheinerss/mcp.json` é criado na raiz do seu repositório:

```json
{
  "mcpServers": {
    "sqlite": {
      "command": "uvx",
      "args": ["mcp-server-sqlite", "--db-path", "./dados.db"],
      "env": {}
    },
    "git": {
      "command": "uvx",
      "args": ["mcp-server-git", "--repository", "."],
      "env": {}
    },
    "meu-servidor-interno": {
      "command": "node",
      "args": ["./scripts/mcp-interno.js"],
      "env": {
        "API_TOKEN": "segredo-local"
      }
    }
  }
}
```

---

## 2. Gerenciamento pelo CLI do Openheinerss

Você pode gerenciar os servidores de ferramentas diretamente através de comandos simples:

### Listar Servidores MCP Configurados
```bash
openheinerss mcp list
```
**Exemplo de Saída**:
```
Configured MCP Servers (.openheinerss/mcp.json):
• git: uvx [mcp-server-git --repository .]
• sqlite: uvx [mcp-server-sqlite --db-path ./dados.db]
```

### Adicionar um Novo Servidor MCP
O comando `openheinerss mcp add` registra automaticamente novos servidores no arquivo de configuração:

```bash
# Adicionar servidor SQLite:
openheinerss mcp add sqlite uvx mcp-server-sqlite --db-path ./dados.db

# Adicionar servidor Git:
openheinerss mcp add git uvx mcp-server-git --repository .

# Adicionar busca web via Brave Search:
openheinerss mcp add brave npx -y @modelcontextprotocol/server-brave-search
```

---

## 3. Como os Motores Compartilham as Ferramentas MCP

Quando uma sessão é iniciada com **qualquer harness**, o Openheinerss faz o roteamento das ferramentas:

1. **Claude Code**: Conecta os servidores definidos em `.openheinerss/mcp.json` ao perfil isolado da sessão.
2. **OpenCode & Aider**: Expõe as ferramentas do MCP central para que o interpretador possa invocar consultas SQL, comandos Git e requisições HTTP locais.
3. **Controle de Segurança Unificado**: Toda chamada gerada por uma ferramenta MCP passa pelo intermediador de segurança do Openheinerss (`pkg/mcp` e `pkg/session`), emitindo `agent.tool_call` e solicitando aprovação humana quando a ferramenta apresentar risco de integridade.
