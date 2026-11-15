package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/repository/sqlcdb"
)

// DBTXBeginner é satisfeito pelo *pgxpool.Pool: queries soltas e transações.
type DBTXBeginner interface {
	sqlcdb.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type BitcoinHistoryRepository struct {
	db DBTXBeginner
	q  *sqlcdb.Queries
}

func NewBitcoinHistoryRepository(db DBTXBeginner) *BitcoinHistoryRepository {
	return &BitcoinHistoryRepository{db: db, q: sqlcdb.New(db)}
}

// Replace troca, numa transação, os pontos da moeda a partir do primeiro ponto
// recebido pelos pontos novos. O que é mais antigo que isso continua salvo.
// pontos deve vir em ordem cronológica e sem instantes repetidos.
func (r *BitcoinHistoryRepository) Replace(ctx context.Context, moeda string, pontos []domain.BitcoinHistoryPoint) error {
	if len(pontos) == 0 {
		return nil
	}

	params := sqlcdb.InsertHistoricoBitcoinParams{
		Moeda:      moeda,
		Momentos:   make([]pgtype.Timestamptz, len(pontos)),
		Precos:     make([]pgtype.Numeric, len(pontos)),
		MarketCaps: make([]pgtype.Numeric, len(pontos)),
		Volumes:    make([]pgtype.Numeric, len(pontos)),
	}
	for i, p := range pontos {
		preco, err := stringToNumeric(p.Price)
		if err != nil {
			return fmt.Errorf("histórico %s %s preço: %w", moeda, p.At, err)
		}
		mc, err := numericOpcional(p.MarketCap)
		if err != nil {
			return fmt.Errorf("histórico %s %s market cap: %w", moeda, p.At, err)
		}
		vol, err := numericOpcional(p.TotalVolume)
		if err != nil {
			return fmt.Errorf("histórico %s %s volume: %w", moeda, p.At, err)
		}
		params.Momentos[i] = pgtype.Timestamptz{Time: p.At, Valid: true}
		params.Precos[i], params.MarketCaps[i], params.Volumes[i] = preco, mc, vol
	}

	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.DeleteHistoricoBitcoinDesde(ctx, sqlcdb.DeleteHistoricoBitcoinDesdeParams{
			Moeda: moeda,
			Desde: pgtype.Timestamptz{Time: pontos[0].At, Valid: true},
		}); err != nil {
			return err
		}
		return q.InsertHistoricoBitcoin(ctx, params)
	})
	if err != nil {
		return fmt.Errorf("salvar histórico do bitcoin (%s): %w", moeda, err)
	}
	return nil
}

// List devolve os pontos da moeda a partir de "desde", em ordem cronológica.
func (r *BitcoinHistoryRepository) List(ctx context.Context, moeda string, desde time.Time) ([]domain.BitcoinHistoryPoint, error) {
	rows, err := r.q.ListHistoricoBitcoin(ctx, sqlcdb.ListHistoricoBitcoinParams{
		Moeda: moeda,
		Desde: pgtype.Timestamptz{Time: desde, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("listar histórico do bitcoin (%s): %w", moeda, err)
	}

	pontos := make([]domain.BitcoinHistoryPoint, 0, len(rows))
	for _, row := range rows {
		preco, err := numericToString(row.Preco)
		if err != nil {
			return nil, fmt.Errorf("histórico %s %s preço: %w", moeda, row.Momento.Time, err)
		}
		mc, err := stringOpcional(row.MarketCap)
		if err != nil {
			return nil, fmt.Errorf("histórico %s %s market cap: %w", moeda, row.Momento.Time, err)
		}
		vol, err := stringOpcional(row.VolumeTotal)
		if err != nil {
			return nil, fmt.Errorf("histórico %s %s volume: %w", moeda, row.Momento.Time, err)
		}
		pontos = append(pontos, domain.BitcoinHistoryPoint{
			At: row.Momento.Time.In(time.UTC), Price: preco, MarketCap: mc, TotalVolume: vol,
		})
	}
	return pontos, nil
}

// LastSync devolve quando a moeda foi sincronizada pela última vez;
// ok = false quando ainda não há nenhum ponto salvo.
func (r *BitcoinHistoryRepository) LastSync(ctx context.Context, moeda string) (time.Time, bool, error) {
	t, err := r.q.GetUltimaSincronizacaoHistoricoBitcoin(ctx, moeda)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("última sincronização do histórico (%s): %w", moeda, err)
	}
	if !t.Valid {
		return time.Time{}, false, nil
	}
	return t.Time.In(time.UTC), true, nil
}

// numericOpcional: nil vira NULL.
func numericOpcional(s *string) (pgtype.Numeric, error) {
	if s == nil {
		return pgtype.Numeric{}, nil
	}
	return stringToNumeric(*s)
}

// stringOpcional: NULL vira nil.
func stringOpcional(n pgtype.Numeric) (*string, error) {
	if !n.Valid {
		return nil, nil
	}
	s, err := numericToString(n)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
