package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/repository/sqlcdb"
)

type BitcoinQuoteRepository struct {
	q *sqlcdb.Queries
}

func NewBitcoinQuoteRepository(db sqlcdb.DBTX) *BitcoinQuoteRepository {
	return &BitcoinQuoteRepository{q: sqlcdb.New(db)}
}

// GetLast devolve a cotação mais recente. Tabela vazia -> domain.ErrNaoEncontrado.
func (r *BitcoinQuoteRepository) GetLast(ctx context.Context) (domain.BitcoinQuote, error) {
	row, err := r.q.GetUltimaCotacaoBitcoin(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BitcoinQuote{}, fmt.Errorf("%w: nenhuma cotação do bitcoin salva", domain.ErrNaoEncontrado)
	}
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("buscar última cotação do bitcoin: %w", err)
	}
	return cotacaoBitcoin(row.ID, row.Usd, row.Brl, row.Fonte, row.ObtidaEm)
}

// Save grava um registro novo (nunca sobrescreve: o histórico alimenta o gráfico).
func (r *BitcoinQuoteRepository) Save(ctx context.Context, q domain.BitcoinQuote) (domain.BitcoinQuote, error) {
	usd, err := stringToNumeric(q.USD)
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("cotação usd: %w", err)
	}
	brl, err := stringToNumeric(q.BRL)
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("cotação brl: %w", err)
	}

	row, err := r.q.CreateCotacaoBitcoin(ctx, sqlcdb.CreateCotacaoBitcoinParams{
		Usd:      usd,
		Brl:      brl,
		Fonte:    q.Source,
		ObtidaEm: pgtype.Timestamptz{Time: q.FetchedAt, Valid: true},
	})
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("salvar cotação do bitcoin: %w", err)
	}
	return cotacaoBitcoin(row.ID, row.Usd, row.Brl, row.Fonte, row.ObtidaEm)
}

func cotacaoBitcoin(id int64, usd, brl pgtype.Numeric, fonte string, obtidaEm pgtype.Timestamptz) (domain.BitcoinQuote, error) {
	u, err := numericToString(usd)
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("cotação %d usd: %w", id, err)
	}
	b, err := numericToString(brl)
	if err != nil {
		return domain.BitcoinQuote{}, fmt.Errorf("cotação %d brl: %w", id, err)
	}
	return domain.BitcoinQuote{ID: id, USD: u, BRL: b, Source: fonte, FetchedAt: obtidaEm.Time.In(time.UTC)}, nil
}
