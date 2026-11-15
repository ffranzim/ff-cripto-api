#!/usr/bin/env bash
# Commita (opcional), publica na VPS e confere as rotas de cotação do Bitcoin.
#
# Uso:
#   scripts/deploy.sh                     # só deploy do diretório atual (como make deploy)
#   scripts/deploy.sh -m "mensagem"       # commita tudo, faz push e depois o deploy
#
# Variáveis opcionais: BASE_URL (padrão: produção), VPS (repassada ao make).
set -euo pipefail

cd "$(dirname "$0")/.."

BASE_URL="${BASE_URL:-https://api.217-76-49-188.sslip.io/cripto}"
MSG=""

while getopts "m:h" opt; do
  case "$opt" in
    m) MSG="$OPTARG" ;;
    h) sed -n '2,8p' "$0"; exit 0 ;;
    *) exit 1 ;;
  esac
done

passo() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }

# 1. Commit + push (só com -m)
if [[ -n "$MSG" ]]; then
  passo "Commit"
  git add -A
  if git diff --cached --quiet; then
    echo "Nada para commitar."
  else
    git commit -m "$MSG"
    passo "Push"
    git push
  fi
elif [[ -n "$(git status --porcelain)" ]]; then
  echo "Aviso: há mudanças não commitadas; elas vão para a VPS mesmo assim (rsync do diretório)."
fi

# 2. Deploy: testes -> rsync -> podman build na VPS -> restart (ver Makefile)
passo "make deploy"
make deploy

# 3. Espera a API subir
passo "Aguardando /healthz"
for i in $(seq 1 30); do
  if curl -fsS -m 5 "$BASE_URL/healthz" >/dev/null 2>&1; then
    echo "API no ar."
    break
  fi
  [[ $i -eq 30 ]] && { echo "API não respondeu em 60s. Veja: make logs"; exit 1; }
  sleep 2
done

# 4. Confere as rotas do Bitcoin
passo "Conferindo rotas"
falhas=0
for rota in /quotes/bitcoin /quotes/bitcoin/coingecko /quotes/bitcoin/binance /quotes/bitcoin/awesomeapi; do
  corpo=$(curl -sS -m 25 -w $'\n%{http_code}' "$BASE_URL$rota" || true)
  status=${corpo##*$'\n'}
  corpo=${corpo%$'\n'*}
  if [[ "$status" == "200" ]]; then
    printf '  OK   %-28s %s\n' "$rota" "$corpo"
  else
    printf '  FAIL %-28s HTTP %s %s\n' "$rota" "$status" "$corpo"
    falhas=$((falhas + 1))
  fi
done

if (( falhas > 0 )); then
  echo
  echo "$falhas rota(s) falharam. 404 = rota não publicada; 503 = provedor fora do ar."
  exit 1
fi
echo
echo "Deploy concluído."
