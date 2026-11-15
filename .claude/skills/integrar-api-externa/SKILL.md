---
name: integrar-api-externa
description: Passo a passo para integrar uma nova API externa (fornecedor de terceiros, ex. Kraken, Mercado Bitcoin, Brapi, Banco Central) nesta API Go, ou adicionar um endpoint novo de um fornecedor que já existe. Use sempre que o usuário pedir para "adicionar um provedor", "integrar a API X", "consumir o endpoint Y", "criar um cliente para", "novo fornecedor", "nova fonte de cotação", ou mencionar uma API de terceiros que ainda não existe em internal/service/externo/. Define onde fica cada arquivo (externo/<fornecedor> vs interno), como reaproveitar httpjson, como testar com httpjsontest e como ligar tudo no main.
---

# Integrar uma API externa

Regra de organização:

```
internal/service/
├── externo/                  # SÓ quem fala HTTP com terceiros
│   ├── <fornecedor>/         # uma pasta (e um pacote) por fornecedor
│   ├── httpjson/             # GET de JSON, erros e validação de preço (comum)
│   │   └── httpjsontest/     # servidor falso + casos de teste (comum)
└── interno/                  # serviços da própria API: cache, TTL, fallback,
                              # singleflight, stale, banco. Nunca fazem HTTP.
```

- `externo/<fornecedor>` não conhece cache, banco, log nem ordem de fallback: busca, valida e devolve tipos de `domain`.
- `interno` não importa `externo`: declara uma interface pequena com o método que precisa, e o `main` injeta o cliente concreto.
- Não crie pasta chamada `internal` dentro de `service` (o Go restringe o import e quebra o `cmd/api`).

Antes de começar, leia um fornecedor parecido como modelo: `externo/binance/` (sem chave, simples), `externo/coingecko/` (chave em header, histórico) ou `externo/awesomeapi/` (vários endpoints no mesmo fornecedor).

## 1. Fornecedor já existe?

Se sim, crie um arquivo novo dentro da pasta dele (ex.: `awesomeapi/moedas.go`) e pule para o passo 3. Se não, siga em ordem.

## 2. Pasta e pacote do fornecedor

- Pasta `internal/service/externo/<fornecedor>/`, nome em minúsculas sem separador (`mercadobitcoin`, `kraken`). Pacote com o mesmo nome.
- Comentário de pacote no arquivo principal: `// Package kraken é o cliente da API da Kraken (o que ele busca).`
- Se o fornecedor for uma fonte de cotação do Bitcoin, adicione a constante `Fonte<Fornecedor>` em `internal/domain/bitcoin.go`.

## 3. O cliente

Padrão (veja `binance/binance.go`):

```go
const url<Fornecedor> = "https://..."

// <Fornecedor> ... (limites, cota, se precisa de chave)
type Kraken struct {
	url    string            // campo, não constante direta: o teste troca pelo httptest
	apiKey string            // só se a API tiver chave; vazia = sem header
	client *http.Client      // recebido de fora, nunca http.DefaultClient
	agora  func() time.Time  // só se o resultado carimba o horário da busca
}

func NewKraken(apiKey string, client *http.Client) *Kraken { ... agora: time.Now }

func (*Kraken) Name() string { return domain.FonteKraken } // se entrar no fallback do BTC

// Formato da resposta em comentário: { "result": { ... } }
type respostaKraken struct { ... }   // tipos da resposta: não exportados

func (k *Kraken) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	var body respostaKraken
	if err := httpjson.Get(ctx, k.client, k.url, k.headers(), &body); err != nil {
		return domain.BitcoinQuote{}, err
	}
	usd, err := httpjson.PrecoPositivo("result.usd", body....)
	...
}
```

Regras:
- Sempre `httpjson.Get` (já põe Accept, limita o corpo a 1 MiB, usa `UseNumber`, devolve `ErrHTTP`/`ErrJSONInvalido`). Não use `http.Get` nem `json.Unmarshal` direto.
- Dinheiro é `string` decimal exata: números do JSON chegam como `json.Number` e passam por `httpjson.PrecoPositivo` (> 0) ou `httpjson.ValorNaoNegativo` (>= 0, ex.: volume). Nunca `float64`.
- Campo faltando -> `fmt.Errorf("%w: <campo>", httpjson.ErrCampoAusente)`; JSON com formato errado -> `httpjson.ErrJSONInvalido`. Embrulhe sempre com `%w`, nunca crie um erro novo para esses casos.
- Parâmetros de query com `url.Values`, caminho com `url.PathEscape`.
- Chave da API só no header, e só se não estiver vazia.
- Datas em UTC (`time.Unix(...).UTC()`, `time.UnixMilli(...).UTC()`).
- Se a API devolver lista, ordene/valide (cronológica, sem repetidos) aqui, não no interno.

## 4. Testes do cliente (`<arquivo>_test.go` no mesmo pacote)

Use `httpjsontest`, sem rede real:

```go
func krakenTeste(srv *httptest.Server) httpjsontest.BitcoinFetcher {
	p := NewKraken("chave-teste", srv.Client())
	p.url, p.agora = srv.URL+"/0/public/Ticker", func() time.Time { return httpjsontest.AgoraTeste }
	return p
}

func TestKraken_Fetch(t *testing.T) {
	httpjsontest.RodarCasos(t, krakenTeste, domain.FonteKraken, []httpjsontest.Caso{
		{Name: "sucesso", Status: 200, Corpo: `...`, WantUSD: "...", WantBRL: "..."},
		{Name: "JSON inválido", Status: 200, Corpo: `<html>`, WantErr: httpjson.ErrJSONInvalido},
		{Name: "campo ausente", ..., WantErr: httpjson.ErrCampoAusente},
		{Name: "preço zero", ..., WantErr: httpjson.ErrPrecoInvalido},
		{Name: "HTTP 429", Status: 429, ..., WantErr: httpjson.ErrHTTP},
	})
}
```

- `RodarCasos` serve para quem implementa `Fetch(ctx) (domain.BitcoinQuote, error)`. Para outros formatos (histórico, moedas), use `httpjsontest.Servidor(t, status, corpo, &req)` e confira na mão, como em `coingecko/historico_test.go`.
- Cubra no mínimo: sucesso com um corpo real copiado da documentação/resposta da API, JSON inválido, campo ausente, valor inválido, status de erro.
- Com chave: um teste que confere o header enviado e outro que confere que, sem chave, o header não vai (`awesomeapi/bitcoin_test.go`).
- Confira a URL/query montada (`req.URL.Path`, `req.URL.RawQuery`).

## 5. O lado interno

Em `internal/service/interno/`:

- Se o dado já tem serviço (ex.: mais um provedor de Bitcoin), não crie nada: o cliente já satisfaz `NamedBitcoinQuoteProvider` / `BitcoinQuoteProvider`.
- Se é um dado novo, crie `<assunto>_service.go` seguindo `currency_quote_service.go`:
  - interface pequena do provider no topo, com comentário `// XProvider é satisfeito pelo *<fornecedor>.<Tipo>.`;
  - cache em memória com TTL, `singleflight` para buscas simultâneas, timeout por busca, `Stale = true` com o último dado bom quando a busca falha, e `domain.ErrIndisponivel` quando não há nada;
  - `agora func() time.Time` para os testes controlarem o relógio.
- Teste com um fake do provider no `_test.go` (como `fakeMoedas`), nunca com o cliente HTTP real.

## 6. Ligar no main e na configuração

- `cmd/api/main.go`: importe `internal/service/externo/<fornecedor>` e injete no serviço `interno`. Cada fornecedor recebe um `*http.Client` com `Timeout` (rede de segurança; o timeout real vem do ctx).
- Provedor de Bitcoin: adicione no mapa `porNome` em `novosServicosBitcoin` **e** em `provedoresBitcoinValidos` (e na mensagem de erro de `parseProvedores`) em `internal/config/config.go`, senão `BTC_QUOTE_PROVIDERS` recusa o nome. Decida com o usuário se ele entra em `provedoresBitcoinPadrao` e em que posição. Atualize `internal/config/config_test.go`.
- Chave de API nova: campo em `config.Config` (`<FORNECEDOR>_API_KEY`, vazio = sem chave, com `log.Warn` no main), e documente em `.env.example`, `deploy/ff-cripto-secret.example.yaml` e na tabela de variáveis do `README.md`.
- Rota nova: interface em `internal/http/router.go`, handler e DTO em `internal/http`, linha na tabela de rotas do `README.md`.
- Documente as chamadas na collection do Bruno em `documentacao/api` seguindo o skill `documentar-api-bruno` (provedor de Bitcoin novo = chamada nova em `Bitcoin/Por fonte/`).

## 7. Conferir

```sh
gofmt -l . && go vet ./... && go test ./...
```

Tudo verde antes de dar como pronto. No resumo para o usuário, mostre a árvore dos arquivos criados.
