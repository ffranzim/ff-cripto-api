# ff-cripto-api

API de cotações: Bitcoin (CoinGecko, Binance, AwesomeAPI com fallback e histórico) e moedas em real
(dólar, euro, libra via AwesomeAPI).

- Produção: `https://api.217-76-49-188.sslip.io/cripto` (Caddy remove o prefixo; a API ouve em `127.0.0.1:8082`)
- Banco `fundos`, schema **`ff_cripto`** (`DB_SCHEMA`): `cotacoes_bitcoin`, `historico_bitcoin`.
- Serviços irmãos: [ff-fundos-api](../ff-fundos-api) (`/fundos`) e [ff-b3-api](../ff-b3-api) (`/b3`).

## Rotas (todas públicas)

| Rota | |
|---|---|
| `GET /healthz` | |
| `GET /quotes/bitcoin` | USD e BRL pela cadeia de provedores (`BTC_QUOTE_PROVIDERS`), cache `BTC_QUOTE_TTL_SECONDS`; grava em `cotacoes_bitcoin` |
| `GET /quotes/bitcoin/{fonte}` | `coingecko` \| `binance` \| `awesomeapi`, sem fallback |
| `GET /quotes/bitcoin/history?currency=usd\|brl&days=30` | `days` 1 a 360: de hora em hora até 90 dias, diário acima (`interval`); CoinGecko, em `historico_bitcoin` |
| `GET /quotes/currencies` | USD, EUR e GBP em BRL |
| `GET /quotes/currencies/history?pair=USD-BRL&days=30` | fechamento diário |

Collection do [Bruno](https://www.usebruno.com) com todas as chamadas em [documentacao/api](documentacao/api)
(ambientes `Local` e `Producao`).

```bash
B=https://api.217-76-49-188.sslip.io/cripto
curl -s $B/quotes/bitcoin
curl -s "$B/quotes/bitcoin/history?currency=brl&days=7"
curl -s $B/quotes/currencies
```

## Configuração

| Variável | Padrão |
|---|---|
| `DATABASE_URL` | obrigatória |
| `DB_SCHEMA` | `ff_cripto` |
| `HOST` / `PORT` | `127.0.0.1` / `8082` |
| `COINGECKO_API_KEY` | vazio = API pública (limite menor) |
| `AWESOMEAPI_API_KEY` | vazio = cota por IP (costuma dar 429 na VPS) |
| `BTC_QUOTE_TTL_SECONDS` | `300` |
| `BTC_QUOTE_PROVIDERS` | `coingecko,binance,awesomeapi` |

## Desenvolvimento e deploy

```bash
make test
make run
make deploy        # testes → rsync ~/apps/ff-cripto-api → podman build → Quadlet ff-cripto → restart
```

O secret `ff-cripto` (DATABASE_URL, COINGECKO_API_KEY, AWESOMEAPI_API_KEY) e a rota `/cripto` no
Caddy vêm do repo de infra (`vps/contabo`). Passo a passo:

- [documentacao/publish/README-publicar-api.md](documentacao/publish/README-publicar-api.md): rodar local (túnel), deploy, conferir, rollback.
- [documentacao/publish/README-vps-contabo.md](documentacao/publish/README-vps-contabo.md): o que preparar na VPS (secret, rota no Caddy, porta).
