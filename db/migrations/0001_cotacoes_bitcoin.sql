-- Histórico das cotações do Bitcoin (uma linha por busca bem-sucedida, para gráfico).
-- usd/brl em numeric (sem float para dinheiro). obtida_em é quando o provedor
-- respondeu; o TTL do cache é contado a partir dela.

-- +goose Up
CREATE TABLE IF NOT EXISTS cotacoes_bitcoin (
  id        bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
  usd       numeric     NOT NULL CHECK (usd > 0),
  brl       numeric     NOT NULL CHECK (brl > 0),
  fonte     text        NOT NULL CHECK (fonte IN ('coingecko', 'binance', 'awesomeapi')),
  obtida_em timestamptz NOT NULL,
  criado_em timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT cotacoes_bitcoin_pkey PRIMARY KEY (id)
);

-- Atende a "última cotação" e o gráfico por período.
CREATE INDEX IF NOT EXISTS cotacoes_bitcoin_obtida_em_idx
  ON cotacoes_bitcoin (obtida_em DESC);

-- +goose Down
DROP TABLE IF EXISTS cotacoes_bitcoin;
