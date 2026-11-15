# VPS Contabo: o que a ff-cripto-api precisa

O que tem que existir **na VPS** (`franzim@217.76.49.188`) para a API funcionar. Isso é configurado no repositório de infra `~/desenv/workspaces/vps/contabo`, não neste.
Depois de pronto, o deploy da API segue [README-publicar-api.md](README-publicar-api.md).

| O quê | Onde (repo de infra) | Por quê |
|---|---|---|
| Secret Podman `ff-cripto` | `scripts/apply-secrets.sh` | o pod lê `DATABASE_URL`, `COINGECKO_API_KEY` e `AWESOMEAPI_API_KEY` dele |
| Rota `/cripto` no Caddy | `caddy/Caddyfile` | publica `127.0.0.1:8082` em `https://api.217-76-49-188.sslip.io/cripto` |
| Porta 8082 livre | — | a API ouve em 127.0.0.1:8082 (8080 = fer, 8081 = b3) |
| Postgres + banco `fundos` | `kube/postgres.yaml` | já existe; o schema `ff_cripto` é criado pela própria API |

## Pré-requisitos

- SSH sem senha: `ssh franzim@217.76.49.188 true` não pode pedir senha.
- No `.env` do repo de infra (fora do git): `POSTGRES_PASSWORD`, `COINGECKO_API_KEY` e, opcional, `AWESOMEAPI_API_KEY` (sem ela a AwesomeAPI dá 429 no IP da VPS).
- Porta 8082 livre:

  ```sh
  ssh franzim@217.76.49.188 'ss -ltn | grep -E ":808[0-9]"'
  ```

## 1. O que fica versionado na infra

`scripts/apply-secrets.sh`, antes do bloco do `fundos-b3`:

```bash
# ff-cripto (repo go/ff-cripto-api): mesmo banco "fundos", tabelas no schema ff_cripto.
FF_CRIPTO_SECRETS=(
  DATABASE_URL "postgres://franzim:$(require POSTGRES_PASSWORD)@127.0.0.1:5432/fundos?sslmode=disable"
  COINGECKO_API_KEY "$(require COINGECKO_API_KEY)"
)
if [[ -n "${ENV[AWESOMEAPI_API_KEY]:-}" ]]; then
  FF_CRIPTO_SECRETS+=(AWESOMEAPI_API_KEY "${ENV[AWESOMEAPI_API_KEY]}")
fi
put_secret ff-cripto "${FF_CRIPTO_SECRETS[@]}"
```

`caddy/Caddyfile`, dentro de `api.{$BASE_DOMAIN} { ... }` e **antes** do `handle` legado (que manda tudo para a 8080):

```
	# cripto: cotações de Bitcoin e moedas (repo go/ff-cripto-api).
	handle_path /cripto/* {
		reverse_proxy 127.0.0.1:8082
	}
```

Validar o Caddyfile antes de enviar (na pasta do repo de infra):

```sh
H=$(printf 'x\nx\n' | podman run --rm -i docker.io/library/caddy:2-alpine caddy hash-password | tail -n1)
podman run --rm -v $PWD/caddy/Caddyfile:/etc/caddy/Caddyfile:ro \
  -e BASE_DOMAIN=x.example -e PUB_TOKEN=t -e PUB_WEB_USER=u -e "PUB_WEB_HASH=$H" \
  docker.io/library/caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
# -> Valid configuration
```

## 2. Aplicar na VPS

Sem o `make deploy` da infra, que reinicia Postgres, unpub e sonarqube à toa:

| # | Onde | Comando | Resultado esperado |
|---|---|---|---|
| 1 | `vps/contabo` | `make sync` | envia `caddy/Caddyfile` e `scripts/apply-secrets.sh` para `~/vps` |
| 2 | `vps/contabo` | `make secrets` | `secret ff-cripto atualizado` (pods em execução não são afetados) |
| 3 | qualquer pasta | `ssh franzim@217.76.49.188 'systemctl --user restart caddy && systemctl --user is-active caddy'` | `active` |
| 4 | qualquer pasta | `curl -s -o /dev/null -w '%{http_code}\n' https://api.217-76-49-188.sslip.io/cripto/healthz` | **502** antes do primeiro deploy da API, 200 depois |

- O Caddyfile é montado como arquivo (`hostPath`): depois do `rsync` o container só enxerga a versão nova reiniciando. Por isso `restart`, e não `caddy reload`. O restart derruba todos os sites por alguns segundos.
- Repita só quando mudar a rota ou uma chave/senha. Trocou uma chave? Depois do `make secrets`, reinicie a API para ela reler o secret: `ssh franzim@217.76.49.188 'systemctl --user restart ff-cripto'`.

## 3. Conferir

```sh
ssh franzim@217.76.49.188 '
  grep -n cripto ~/vps/caddy/Caddyfile
  podman secret ls --format "{{.Name}}" | grep -x ff-cripto
  systemctl --user is-active caddy ff-cripto
  ss -ltn | grep ":8082 "'
```

| Sintoma | Causa |
|---|---|
| 404 `{"erro":{"codigo":"nao_encontrado",...}}` em `/cripto/...` | rota `/cripto` ausente no Caddyfile da VPS ou Caddy não reiniciado. A resposta vem da fiis-rendimentos-api pelo `handle` legado, que usa o mesmo formato de erro |
| 502 | rota ok; a API não está em 127.0.0.1:8082 (ainda não publicada ou caiu) |
| Pod `ff-cripto` não sobe, log fala de secret | `make secrets` não rodou |

## Túnel para o Postgres

O Postgres só escuta em `127.0.0.1:5432` na VPS. Do PC, abra o túnel e deixe o terminal aberto:

```sh
make db-tunnel       # no repo de infra
make tunnel          # ou neste repo (mesmo comando)
```
