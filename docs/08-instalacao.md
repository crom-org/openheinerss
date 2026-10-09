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

## Atualizar o que está instalado e rodando

```bash
openheinerss atualizar --seco          # mostra o plano, sem alterar nada
openheinerss atualizar                 # compila do clone (se a pasta atual for o repositório) ou baixa a última release
openheinerss atualizar --release       # força a release do GitHub (confere o checksum publicado)
openheinerss atualizar --de-fonte --repo ~/openheinerss
openheinerss atualizar --voltar        # restaura <destino>.anterior e reinicia os serve
```

O binário é gravado em arquivo temporário na mesma pasta e movido com `rename` (atômico); o anterior fica em
`~/.local/bin/openheinerss.anterior` (`--destino` troca o caminho). Em Linux, os `openheinerss serve` do usuário
que usam esse binário são encerrados com SIGTERM e relançados desacoplados (`setsid`) com os mesmos argumentos,
ambiente e pasta, e o comando confere que a porta voltou a responder; `--sem-reiniciar` pula essa etapa. Agentes
`rodar` em andamento nunca são tocados: o comando só lista quais continuam na versão antiga. `serve --stdio` também
não é reiniciado (depende do processo pai). Em outros sistemas não há reinício automático. `--json` devolve o
plano/resultado para a Central.

## Go

Quem já usa o toolchain Go pode instalar a versão publicada do módulo:

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
