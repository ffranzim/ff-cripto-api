-- name: DeleteHistoricoBitcoinDesde :exec
DELETE FROM historico_bitcoin
WHERE moeda = sqlc.arg('moeda') AND momento >= sqlc.arg('desde');

-- name: InsertHistoricoBitcoin :exec
-- Insere todos os pontos num único comando (um array por coluna).
INSERT INTO historico_bitcoin (moeda, momento, preco, market_cap, volume_total)
SELECT sqlc.arg('moeda')::text,
       unnest(sqlc.arg('momentos')::timestamptz[]),
       unnest(sqlc.arg('precos')::numeric[]),
       unnest(sqlc.arg('market_caps')::numeric[]),
       unnest(sqlc.arg('volumes')::numeric[]);

-- name: ListHistoricoBitcoin :many
SELECT momento, preco, market_cap, volume_total
FROM historico_bitcoin
WHERE moeda = sqlc.arg('moeda') AND momento >= sqlc.arg('desde')
ORDER BY momento;

-- name: GetUltimaSincronizacaoHistoricoBitcoin :one
-- NULL quando ainda não há pontos daquela moeda.
SELECT max(criado_em)::timestamptz AS sincronizado_em
FROM historico_bitcoin
WHERE moeda = sqlc.arg('moeda');
