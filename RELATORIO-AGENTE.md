# Relatório do agente

- Corrigida a resolução da pasta de configuração: `XDG_CONFIG_HOME` explícito
  agora é respeitado também no macOS e Windows; sem ele, permanece o caminho
  nativo de `os.UserConfigDir`. Aplicado a comandos, contexto, MCP e risco.
- Decisão: corrigir o código, pois os testes usam `XDG_CONFIG_HOME` para isolar
  a configuração e esse contrato deve ser portátil entre sistemas.
- Verificações: `go build ./...`, `go vet ./...`, `go test ./...`,
  `go test -count=2 ./...` e `go test -race ./...` passaram (22 pacotes; 0
  falhas). `go run ./cmd/openheinerss docs --check` também passou.
