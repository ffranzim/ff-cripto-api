# Publicar a ff-cripto-api

O que fazer **neste repositório** para rodar a API localmente e publicá-la na VPS da Contabo.
O que precisa existir na VPS antes (secret, rota no Caddy, porta) está em [README-vps-contabo.md](README-vps-contabo.md).

```
internet ──https──> Caddy (api.217-76-49-188.sslip.io)
                     └─ /cripto/*  ──(remove o prefixo)──> 127.0.0.1:8082  pod ff-cripto
                                                              └─ 127.0.0.1:5432  Postgres (banco fundos, schema ff_cripto)
```

Arquivos deste repo envolvidos no deploy:

| Arquivo | Papel |
|---|---|
| `Containerfile` | build da imagem `localhost/ff-cripto:latest` (feito na VPS) |
| `deploy/ff-cripto.yaml` | pod: init container `migrate` + API em 127.0.0.1:8082, lê o secret `ff-cripto` |
| `deploy/ff-cripto.kube` | Quadlet: vira a unidade systemd `ff-cripto` |
| `Makefile` | `tunnel`, `run`, `migrate`, `deploy`, `status`, `logs` |
| `scripts/deploy.sh` | `make deploy` + espera o `/healthz` + confere as rotas |

Todos os comandos rodam **no seu PC**, na raiz deste repo; os `make` fazem o SSH.

## Resumo

| # | Comando | Resultado esperado |
|---|---|---|
| 0 | VPS preparada ([README-vps-contabo.md](README-vps-contabo.md)) | `/cripto/healthz` dá **502** antes do primeiro deploy |
| 1 | `scripts/deploy.sh` | `OK` nas 4 rotas do Bitcoin e `Deploy concluído.` |
| 2 | `cd documentacao/api && npx -y @usebruno/cli run -r --env Producao` | todas as chamadas passam |

Nas publicações seguintes, só o passo 1.

## 1. Rodar localmente (com túnel)

O Postgres da VPS só escuta em `127.0.0.1:5432` **lá**. Abra o túnel SSH e deixe o terminal aberto:

```sh
make tunnel          # = ssh -N -L 5432:127.0.0.1:5432 franzim@217.76.49.188
```

Em outro terminal:

```sh
make migrate         # aplica as migrations no schema ff_cripto (via túnel)
make run             # API em http://127.0.0.1:8082 usando o .env
```

- Use `PORT=8082` no `.env` (como no `.env.example`): bate com o ambiente **Local** do Bruno e não colide com a fiis-rendimentos-api (8080). `ECONNREFUSED 127.0.0.1:8082` no Bruno = API não está rodando nessa porta.
- Se a 5432 do seu PC já estiver ocupada (Postgres local): `ssh -N -L 15432:127.0.0.1:5432 franzim@217.76.49.188` e troque a porta no `DATABASE_URL` do `.env`.
- `make run` seguido de Ctrl+C mostra `make: *** Error 1`: é normal com `go run` (a linha `sinal recebido, encerrando` confirma que desligou certo).
- **Cuidado:** pelo túnel você está no **banco de produção**. `make run` grava cotações de verdade em `ff_cripto`.

Validar a collection contra a API local:

```sh
cd documentacao/api && npx -y @usebruno/cli run -r --env Local
```

## 2. Deploy

```sh
scripts/deploy.sh                 # make deploy + espera o /healthz + confere as rotas do Bitcoin
scripts/deploy.sh -m "mensagem"   # commita, faz push e depois o deploy
```

O `make deploy` (chamado pelo script):

1. roda `go test ./...` (falhou = não publica);
2. `rsync` do diretório para `~/apps/ff-cripto-api` na VPS (sem `.git`, `.env`, `bin`);
3. `podman build -t localhost/ff-cripto:latest .` **na VPS**;
4. instala `deploy/ff-cripto.kube` e `deploy/ff-cripto.yaml` em `~/.config/containers/systemd/`;
5. `systemctl --user daemon-reload && systemctl --user restart ff-cripto`.

No start, o init container roda `api migrate` (cria o schema `ff_cripto` se preciso e aplica as migrations) antes de subir a API.

Variável nova de ambiente? Ela precisa estar em `deploy/ff-cripto.yaml` **e** no secret `ff-cripto` (veja [README-vps-contabo.md](README-vps-contabo.md)).

## 3. Conferir

```sh
make status                                         # pod e unidade systemd
make logs                                           # últimas 100 linhas do journal
curl -s https://api.217-76-49-188.sslip.io/cripto/healthz
curl -s https://api.217-76-49-188.sslip.io/cripto/quotes/bitcoin
cd documentacao/api && npx -y @usebruno/cli run -r --env Producao
```

| Sintoma | Causa provável |
|---|---|
| 404 `{"erro":{"codigo":"nao_encontrado",...}}` em `/cripto/...` | não é esta API: falta a rota `/cripto` no Caddy ([README-vps-contabo.md](README-vps-contabo.md)) e a requisição caiu na fiis-rendimentos-api |
| 502 | Caddy ok, mas nada em 127.0.0.1:8082: `make status` / `make logs` |
| Pod não sobe, log fala de secret | secret `ff-cripto` não existe ([README-vps-contabo.md](README-vps-contabo.md)) |
| 503 em `/healthz` | API sem Postgres (`DATABASE_URL` errado no secret) |
| 503 em `/quotes/bitcoin/{fonte}` | provedor externo fora/limitado (ex.: AwesomeAPI 429 sem chave) |

## 4. Rollback

A imagem é sempre `:latest`, então o rollback é publicar o código anterior:

```sh
git checkout <commit-bom> && make deploy && git checkout main
```

Parar a API sem remover: `ssh franzim@217.76.49.188 'systemctl --user stop ff-cripto'`.
