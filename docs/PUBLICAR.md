# Publicação da versão 1.7.0

Checklist para o mantenedor. Os comandos abaixo apenas preparam e conferem os pacotes; não fazem upload.

## Verificações locais

```bash
go build ./...
go vet ./...
go test -count=1 ./...
flock /tmp/crom-pesado.lock go test -race ./...
bin/openheinerss docs --check
```

Confira que o binário, `sdk/typescript/package.json`, `sdk/python/pyproject.toml` e `sdk/php/composer.json` estão em `1.7.0`.

## TypeScript/npm

```bash
cd sdk/typescript
npm ci
npm run build
npm test
npm pack --dry-run
npm publish --access public
```

Revise a lista do `npm pack --dry-run`: somente `dist/` e `README.md` devem entrar. O último comando é o upload e só deve ser executado após aprovação explícita.

## Python/PyPI

```bash
cd sdk/python
python -m build
twine check dist/*
python -m build
twine upload dist/*
```

`python -m build` é a conferência do conteúdo do pacote e não publica nada. Inspecione `dist/` para garantir que não há testes, caches, caminhos locais ou segredos. `twine upload` é o upload e deve ser executado somente após aprovação.

## Packagist

O SDK PHP é publicado pelo repositório Composer. Confira o `composer.json`, a versão `1.7.0` e o conteúdo versionado; depois, se necessário, use o webhook do Packagist para atualizar o pacote. Não há comando de upload local neste projeto.

## Tag e release

A tag/release é automatizada pelo workflow de release. A versão `1.7.0` já está tagueada: não crie outra tag. Após uma aprovação separada, o fluxo pode ser acionado conforme a configuração do provedor; não faça push nesta etapa.
