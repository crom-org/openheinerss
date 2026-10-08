# Instalação

## Script recomendado

O script baixa o último release de `crom-org/openheinerss`, escolhe o sistema e a arquitetura atuais, verifica `checksums.txt` e instala em `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/crom-org/openheinerss/main/install.sh | bash
```

Uma versão específica pode ser fixada (com ou sem `v`):

```bash
curl -fsSL https://raw.githubusercontent.com/crom-org/openheinerss/main/install.sh | bash -s -- --versao 0.1.0
```

Dentro de um clone, `./install.sh --local` ignora o release e compila o código do clone, injetando versão, commit e data no binário. O destino padrão é `~/.local/bin`; para testar sem alterar a instalação global, use `OPENHEINERSS_INSTALL_DIR=$(pwd)/bin ./install.sh --local`. O alvo `make build` deixa o binário em `bin/openheinerss`. Se não houver rede ou release disponível, o script compila `./cmd/openheinerss` quando executado dentro do clone do projeto.

Quem usa o binário global precisa reinstalá-lo para receber esta versão (`./install.sh --local` ou o instalador de release). O binário global não é alterado por `make build`; executar `bin/openheinerss` usa sempre o build local.

## Go

Quem já usa o toolchain Go pode instalar a versão publicada do módulo (enquanto a próxima versão não for publicada, `@latest` é a v1.0.0, sem as novidades do `CHANGELOG.md`):

```bash
go install github.com/crom-org/openheinerss/cmd/openheinerss@latest
```

## Binário manual

Na página de releases, baixe o artefato correspondente ao sistema e à arquitetura. Os nomes são estáveis:
`openheinerss_<versão>_<sistema>_<arquitetura>.tar.gz`, ou `.zip` no Windows. Baixe também `checksums.txt`, valide o SHA-256, extraia `openheinerss` e coloque-o em um diretório do `PATH`.

Por fim, confirme:

```bash
openheinerss version
```

Um `go build` direto também mostra o commit e a data quando o Go consegue registrar `vcs.revision` e `vcs.time`; `make build` e `install.sh --local` injetam explicitamente os três valores. Releases informam os valores injetados pelo GoReleaser.
