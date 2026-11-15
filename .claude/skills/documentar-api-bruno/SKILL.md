---
name: documentar-api-bruno
description: Mantém a collection do Bruno em documentacao/api sincronizada com as rotas da API Go. Use sempre que uma rota for criada, alterada ou removida em internal/http/router.go, quando mudar parâmetros, DTOs (internal/http/dto.go) ou códigos de erro, quando entrar um provedor novo em /quotes/bitcoin/{fonte}, ou quando o usuário pedir para "documentar a rota", "atualizar o bruno", "criar a chamada no bruno", "atualizar a collection" ou "documentação da api". Define estrutura de pastas, formato dos arquivos .bru, asserts e docs.
---

# Documentar a API no Bruno

A documentação das rotas é uma collection do Bruno em `documentacao/api/`. Ela é a fonte de verdade de "como chamar a API": toda rota em `internal/http/router.go` tem pelo menos uma chamada lá.

## Estrutura

```
documentacao/api/
├── bruno.json                 # name: ff-cripto-api (não mexer)
├── collection.bru             # docs gerais: o que cada pasta representa, formato de erro
├── environments/
│   ├── Local.bru              # baseUrl: http://127.0.0.1:8082
│   └── Producao.bru           # baseUrl: https://api.217-76-49-188.sslip.io/cripto
├── Health/
├── Bitcoin/                   # serviços de internal/service/interno
│   └── Por fonte/             # um provedor de internal/service/externo/<fornecedor>, sem fallback
└── Moedas/
```

- Uma pasta por assunto (o mesmo agrupamento dos serviços em `interno`), cada uma com `folder.bru`. Assunto novo = pasta nova com `seq` seguinte.
- Chamadas que vão direto a um fornecedor externo ficam numa subpasta `Por fonte/` do assunto.
- Nome do arquivo sem acento (`Historico BRL.bru`); o `name` do `meta` pode ter acento (`Histórico BRL`).
- Casos de erro que valem documentar (parâmetro obrigatório, fonte desconhecida) são chamadas próprias, com o status no nome: `Historico sem pair (400).bru`.
- `seq` define a ordem na barra lateral: caminho feliz primeiro, erros por último.

## Formato

`folder.bru`:

```
meta {
  name: Moedas
  seq: 3
}
```

Chamada (`<Nome>.bru`), sempre com `{{baseUrl}}`, nunca URL fixa:

```
meta {
  name: Histórico USD-BRL
  type: http
  seq: 2
}

get {
  url: {{baseUrl}}/quotes/currencies/history?pair=USD-BRL&days=30
  body: none
  auth: none
}

params:query {
  pair: USD-BRL
  days: 30
}

assert {
  res.status: eq 200
  res.body.pair: eq USD-BRL
  res.body.dados: isArray
  res.body.stale: isBoolean
}

docs {
  O que a rota faz, em 1 linha.

  - `pair`: obrigatório, `USD-BRL | EUR-BRL | GBP-BRL`.
  - `days`: 1 a 30 (padrão 30).
  - Cache, TTL e quando volta `stale: true` ou 503.
}
```

Regras:
- A query aparece **nos dois lugares**: na `url` e em `params:query`, com os mesmos valores (o Bruno usa os dois).
- `assert`: **cada chave só uma vez por bloco** (chave repetida quebra o arquivo). Se quiser checar tipo e valor, fique só com o valor (`eq coingecko` em vez de `isString`).
- Os nomes dos campos nos asserts vêm das tags `json:"..."` de `internal/http/dto.go`, nunca de suposição. Números decimais (`json.Number`) são `isNumber`; datas são `isString`.
- Erros: `res.status: eq <código>`, `res.body.erro.codigo: eq <codigo>` e `res.body.erro.mensagem: isString` (formato de `writeError` em `internal/http/respond.go`).
- `docs`: em português, curto. Diga parâmetros (obrigatório? faixa? padrão?), de onde vem o dado (qual fornecedor/tabela), cache/TTL, quando volta `stale` e quais status de erro existem. Tire isso dos comentários do handler em `internal/http/handlers.go` e do service em `internal/service/interno`.
- Variável nova de ambiente (ex.: token) vai em **todos** os arquivos de `environments/`. Segredo de verdade nunca vai no arquivo: use `vars:secret [ nome ]`.

## Passo a passo

1. Leia `internal/http/router.go` e liste as rotas registradas. Compare com `grep -rh "url:" documentacao/api` para achar o que falta, mudou ou sobrou.
2. Para cada rota nova/alterada, leia o handler (`handlers.go`), o DTO (`dto.go`) e o service correspondente.
3. Crie ou edite os `.bru` seguindo o formato acima. Rota removida: apague o arquivo.
4. Rota com parâmetro de caminho que tem valores fixos (ex.: `{fonte}`): uma chamada por valor válido + uma com valor inválido.
5. Se surgiu pasta nova ou mudou o significado de uma, atualize `collection.bru`.
6. Mantenha a tabela de rotas do `README.md` coerente com a collection.
7. Confira que não há chave repetida nos asserts:

   ```sh
   find documentacao/api -name '*.bru' -print0 | xargs -0 awk '/^assert \{/{a=1;next} a&&/^\}/{a=0} a{split($0,k,":"); if(seen[FILENAME,k[1]]++) print FILENAME": duplicado "k[1]}'
   ```

8. Se a API estiver rodando (`make run`, precisa do Postgres), valide de verdade:

   ```sh
   cd documentacao/api && npx -y @usebruno/cli run --env Local
   ```

   Sem API de pé, diga ao usuário que os asserts não foram executados.
