# Relatório — identidade de conta

- Implementado `identidade` efetiva no `session.create`, no RPC `instancia.identidade` e na CLI `openheinerss identidade <instancia> [--json]`.
- `contaId` usa os 16 primeiros hex do SHA-256 do diretório absoluto, limpo e resolvido por symlink; nomes de instância não diferenciam maiúsculas/minúsculas. O modo retornado agora é o modo real do adaptador.
- SDKs TypeScript, Python e PHP expõem a identidade da sessão e `identidade(instancia)`. Protocolo, SDKs, CLI gerado e CHANGELOG foram atualizados.
- Testes de identidade usam apenas diretórios temporários e cobrem symlink, nomes equivalentes e pastas diferentes.

Verificações: `go build ./...` OK; `go vet ./...` OK; `go test ./...` OK; `go test -count=2 ./...` OK (23 pacotes); `go test -race ./...` OK; `docs --check` OK; Python 23 testes OK; TypeScript 7 testes OK; PHP integração OK.
