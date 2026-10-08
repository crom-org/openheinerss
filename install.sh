#!/usr/bin/env bash
set -euo pipefail

repo="crom-org/openheinerss"
versao=""
local=0
base_url="${OPENHEINERSS_BASE_URL:-}"
destino="${OPENHEINERSS_INSTALL_DIR:-${HOME}/.local/bin}"

uso() {
  cat <<'EOF'
Uso: install.sh [--versao VERSAO] [--local]

Instala o binário Openheinerss em ~/.local/bin. Com --local, compila o clone
atual (go build) em vez de baixar o release. A versão pode ser informada
com ou sem o prefixo v. OPENHEINERSS_BASE_URL permite testar um diretório de
artefatos local (por exemplo, um snapshot servido por HTTP).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --versao)
      [[ $# -ge 2 ]] || { echo "Erro: --versao exige um valor." >&2; exit 2; }
      versao="$2"
      shift 2
      ;;
    --local)
      local=1
      shift
      ;;
    -h|--help)
      uso
      exit 0
      ;;
    *)
      echo "Erro: opção desconhecida: $1" >&2
      uso >&2
      exit 2
      ;;
  esac
done

versao="${versao#v}"
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) os="windows" ;;
  *) echo "Erro: sistema operacional não suportado: ${os}" >&2; exit 1 ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Erro: arquitetura não suportada: ${arch}" >&2; exit 1 ;;
esac

tem_baixador=0
if command -v curl >/dev/null 2>&1; then
  baixar() { curl --fail --location --silent --show-error "$1"; }
  tem_baixador=1
elif command -v wget >/dev/null 2>&1; then
  baixar() { wget --quiet --output-document=- "$1"; }
  tem_baixador=1
fi

fallback_local() {
  if [[ -f go.mod && -d cmd/openheinerss ]]; then
    [[ "$local" -eq 1 ]] || echo "Release indisponível; compilando o Openheinerss no repositório atual..." >&2
    mkdir -p "$destino"
    go build -o "$destino/openheinerss" ./cmd/openheinerss
    chmod 0755 "$destino/openheinerss"
    echo "Instalado em $destino/openheinerss (build local)."
    exit 0
  fi
  return 1
}

if [[ "$local" -eq 1 ]]; then
  fallback_local || { echo "Erro: --local exige executar o script na raiz do clone (go.mod e cmd/openheinerss)." >&2; exit 1; }
fi

if [[ "$tem_baixador" -eq 0 ]]; then
  fallback_local || { echo "Erro: curl/wget não encontrado e não há repositório Go para fallback." >&2; exit 1; }
fi

if [[ -z "$versao" && -n "$base_url" ]]; then
  echo "Erro: informe --versao ao usar OPENHEINERSS_BASE_URL." >&2
  fallback_local || exit 1
fi

if [[ -z "$versao" ]]; then
  tag="$(baixar "https://api.github.com/repos/${repo}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)" || true
  versao="${tag#v}"
fi

if [[ -z "$versao" ]]; then
  echo "Não foi possível descobrir o último release." >&2
  fallback_local || exit 1
fi

arquivo="openheinerss_${versao}_${os}_${arch}"
if [[ "$os" == windows ]]; then
  pacote="${arquivo}.zip"
else
  pacote="${arquivo}.tar.gz"
fi
if [[ -n "$base_url" ]]; then
  base_url="${base_url%/}"
else
  base_url="https://github.com/${repo}/releases/download/v${versao}"
fi
url="${base_url}/${pacote}"
checksums_url="${base_url}/checksums.txt"
temporario="$(mktemp -d)"
trap 'rm -rf "$temporario"' EXIT

if ! baixar "$url" >"$temporario/$pacote" || ! baixar "$checksums_url" >"$temporario/checksums.txt"; then
  echo "Não foi possível baixar o release ${versao} (${os}/${arch})." >&2
  fallback_local || exit 1
fi

linha="$(grep -E "[[:space:]]${pacote}$" "$temporario/checksums.txt" || true)"
if [[ -z "$linha" ]]; then
  echo "Checksum ausente para $pacote." >&2
  fallback_local || exit 1
fi
esperado="${linha%% *}"
if command -v sha256sum >/dev/null 2>&1; then
  calculado="$(sha256sum "$temporario/$pacote" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  calculado="$(shasum -a 256 "$temporario/$pacote" | awk '{print $1}')"
else
  echo "Erro: sha256sum/shasum não encontrado." >&2
  exit 1
fi
[[ "$calculado" == "$esperado" ]] || { echo "Checksum inválido para $pacote." >&2; exit 1; }

mkdir -p "$temporario/conteudo"
case "$pacote" in
  *.tar.gz) tar -xzf "$temporario/$pacote" -C "$temporario/conteudo" ;;
  *.zip) unzip -q "$temporario/$pacote" -d "$temporario/conteudo" ;;
esac
binario="$temporario/conteudo/openheinerss"
[[ "$os" == windows ]] && binario="${binario}.exe"
[[ -x "$binario" || -f "$binario" ]] || { echo "O pacote não contém o binário esperado." >&2; exit 1; }
mkdir -p "$destino"
install -m 0755 "$binario" "$destino/openheinerss${os:+}" 2>/dev/null || cp "$binario" "$destino/openheinerss"
chmod 0755 "$destino/openheinerss"
echo "Instalado openheinerss ${versao} em $destino/openheinerss."
