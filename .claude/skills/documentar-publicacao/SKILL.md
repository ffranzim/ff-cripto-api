---
name: documentar-publicacao
description: Cria ou atualiza a documentação de publicação em documentacao/publish (README-publicar-api.md para o que se faz neste repo e README-vps-contabo.md para o que precisa existir na VPS da Contabo). Use quando o usuário pedir "documentação de deploy", "como publicar", "passo a passo para publicar na contabo/vps", "documentacao publish", "readme de publicação", ou quando mudar algo que afeta o deploy (porta, secret, variável de ambiente, rota no Caddy, Containerfile, deploy/*.yaml, Makefile, scripts/deploy.sh).
---

# Documentar a publicação

São dois arquivos em `documentacao/publish/`, um para cada repositório que precisa mexer. Não junte os dois.

| Arquivo | Para quem | Conteúdo |
|---|---|---|
| `README-publicar-api.md` | quem trabalha **neste repo** | rodar local com túnel, deploy, conferir, sintomas, rollback |
| `README-vps-contabo.md` | o que se faz no **repo de infra** (`~/desenv/workspaces/vps/contabo`) | secret, rota no Caddy, porta, como aplicar sem derrubar o resto, conferir |

Os dois se linkam, e o `README.md` da raiz linka os dois (seção de deploy).

## 1. Levantar os fatos (nunca inventar)

Leia antes de escrever:

- **Este repo:** `Makefile` (alvos `tunnel`, `run`, `migrate`, `deploy`, `status`, `logs`, variáveis `VPS`, `REMOTE_DIR`, `IMAGE`), `scripts/deploy.sh` (rotas que ele confere), `Containerfile`, `deploy/*.kube`, `deploy/*.yaml` (porta, `hostNetwork`, init container de migrate, chaves do secret, `optional: true`), `deploy/*-secret.example.yaml`, `.env.example`, `internal/config/config.go` (variáveis e padrões).
- **Repo de infra (só leitura):** `Makefile` (`sync`, `secrets`, `db-tunnel`, o que o `deploy` reinicia), `scripts/apply-secrets.sh` (existe `put_secret <nome-do-secret>`?), `caddy/Caddyfile` (existe `handle_path /<prefixo>/*`? está antes do `handle` legado?), `kube/caddy.yaml` (Caddyfile montado via `hostPath`, por isso usa-se `restart`, não `reload`).
- **VPS (só leitura, por SSH):**

  ```sh
  ssh -o BatchMode=yes franzim@217.76.49.188 '
    grep -n "handle_path\|reverse_proxy" ~/vps/caddy/Caddyfile
    podman secret ls --format "{{.Name}}"
    ss -ltn | grep -E ":80[0-9][0-9] |:5432 "
    podman pod ps --format "{{.Name}} {{.Status}}"'
  curl -s -o /dev/null -w '%{http_code}\n' https://api.217-76-49-188.sslip.io/<prefixo>/healthz
  ```

  Use isso para dizer o que já está pronto e o que falta, e para escolher uma porta livre.

## 2. `README-publicar-api.md`

Seções, nesta ordem:

1. Título + uma linha dizendo que é o que se faz neste repo, com link para o `README-vps-contabo.md`.
2. Diagrama em texto: `internet → Caddy (/<prefixo>/*, remove o prefixo) → 127.0.0.1:<porta> pod → 127.0.0.1:5432 Postgres (banco, schema)`.
3. Tabela dos arquivos de deploy do repo e o papel de cada um.
4. **Resumo**: tabela `# | Comando | Resultado esperado` (VPS pronta = `/<prefixo>/healthz` dá 502; `scripts/deploy.sh`; Bruno `--env Producao`). Diga que nas publicações seguintes basta o deploy.
5. **Rodar localmente (com túnel)**: `make tunnel` num terminal; `make migrate` e `make run` no outro. Inclua:
   - `PORT` do `.env` igual à porta do pod e do ambiente Local do Bruno; `ECONNREFUSED 127.0.0.1:<porta>` = API não está nessa porta;
   - porta 5432 local ocupada → túnel em outra porta e ajuste do `DATABASE_URL`;
   - `make: *** Error 1` depois de Ctrl+C no `go run` é normal;
   - aviso em negrito: pelo túnel é o **banco de produção**.
6. **Deploy**: `scripts/deploy.sh` (e `-m`), e a lista numerada do que o `make deploy` faz (copiada do Makefile real). Migrations no init container. Variável nova = `deploy/*.yaml` **e** secret na infra.
7. **Conferir**: `make status`, `make logs`, `curl`, Bruno; tabela de sintomas (404 com JSON de erro em `/<prefixo>/...` = rota ausente no Caddy e a resposta vem de outra API pelo `handle` legado; 502 = nada na porta; pod não sobe por secret; 503 no `/healthz` = banco; 503 por fonte = fornecedor externo).
8. **Rollback**: `git checkout <commit-bom> && make deploy && git checkout main`; como parar o serviço.

## 3. `README-vps-contabo.md`

1. Título + linha dizendo que é configurado no repo de infra, com link de volta.
2. Tabela `O quê | Onde (repo de infra) | Por quê`: secret, rota no Caddy, porta livre (liste as já usadas), Postgres/banco.
3. **Pré-requisitos**: SSH sem senha, chaves no `.env` da infra, conferir porta livre.
4. **O que fica versionado na infra**: o trecho exato de `apply-secrets.sh` (`put_secret <nome> ...`, chaves opcionais com `if`) e do `Caddyfile` (`handle_path /<prefixo>/* { reverse_proxy 127.0.0.1:<porta> }`, **antes** do `handle` legado), e o comando de `caddy validate` com hash bcrypt de verdade (hash falso dá erro de base64).
5. **Aplicar na VPS**: tabela `# | Onde | Comando | Resultado esperado`: `make sync`, `make secrets`, `systemctl --user restart caddy` (não use o `make deploy` da infra: reinicia Postgres/unpub/sonarqube), `curl` esperando 502 antes do primeiro deploy. Explique o `restart` vs `reload` (hostPath + rsync troca o inode) e que o restart derruba os sites por segundos. Trocou chave → `make secrets` + restart do serviço da API.
6. **Conferir**: um `ssh` que mostra rota, secret, `is-active` e porta; tabela de sintomas.
7. **Túnel**: `make db-tunnel` (infra) ou `make tunnel` (este repo).

## Regras

- Comandos sempre copiáveis, com host, porta, prefixo e nomes reais tirados dos arquivos. Nada de `<placeholder>` no texto final, a não ser `<commit-bom>`.
- Se o Makefile não tiver `tunnel`, adicione: `tunnel: ## Postgres da VPS em localhost:5432` → `ssh -N -L 5432:127.0.0.1:5432 $(VPS)`.
- Não altere o repo de infra a partir desta sessão (um hook bloqueia). Se faltar secret ou rota lá, documente no `README-vps-contabo.md` e entregue ao usuário um prompt autocontido para colar numa sessão aberta em `~/desenv/workspaces/vps/contabo`.
- Não rode `make deploy`/`scripts/deploy.sh` nem reinicie serviços na VPS: isso é do usuário. Comandos de SSH só de leitura.
- Garanta que `documentacao/` está no `.containerignore`.
- No fim, diga ao usuário o que já está pronto na VPS e qual é o próximo passo dele.
