-- name: GetUltimaCotacaoBitcoin :one
SELECT id, usd, brl, fonte, obtida_em
FROM cotacoes_bitcoin
ORDER BY obtida_em DESC, id DESC
LIMIT 1;

-- name: CreateCotacaoBitcoin :one
INSERT INTO cotacoes_bitcoin (usd, brl, fonte, obtida_em)
VALUES (sqlc.arg('usd'), sqlc.arg('brl'), sqlc.arg('fonte'), sqlc.arg('obtida_em'))
RETURNING id, usd, brl, fonte, obtida_em;
