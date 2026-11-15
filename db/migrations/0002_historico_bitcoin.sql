-- Histórico de preço do Bitcoin para gráfico, vindo do market_chart da CoinGecko
-- (pontos de hora em hora nos últimos 30 dias), em USD e BRL.
-- Cada sincronização substitui a janela que a CoinGecko devolveu; o que é mais
-- antigo que ela continua salvo. criado_em marca a sincronização (TTL do cache).

-- +goose Up
CREATE TABLE IF NOT EXISTS historico_bitcoin (
  moeda        text        NOT NULL CHECK (moeda IN ('usd', 'brl')),
  momento      timestamptz NOT NULL,
  preco        numeric     NOT NULL CHECK (preco > 0),
  market_cap   numeric              CHECK (market_cap >= 0),
  volume_total numeric              CHECK (volume_total >= 0),
  criado_em    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT historico_bitcoin_pkey PRIMARY KEY (moeda, momento)
);

-- +goose Down
DROP TABLE IF EXISTS historico_bitcoin;
