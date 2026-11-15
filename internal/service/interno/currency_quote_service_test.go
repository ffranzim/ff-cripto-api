package interno

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

type fakeMoedas struct {
	err      error
	chamadas atomic.Int32
}

func (f *fakeMoedas) FetchCurrencies(context.Context) ([]domain.CurrencyQuote, error) {
	f.chamadas.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return []domain.CurrencyQuote{{Code: "USD", CodeIn: "BRL", Bid: "5.5"}}, nil
}

func TestMoedasCacheStaleEIndisponivel(t *testing.T) {
	prov := &fakeMoedas{}
	agora := agoraBTC
	s := NewCurrencyQuoteService(prov, "awesomeapi", ttlTeste, time.Second)
	s.agora = func() time.Time { return agora }
	ctx := context.Background()

	r, err := s.GetQuotes(ctx)
	if err != nil || r.Stale || len(r.Quotes) != 1 || r.Source != "awesomeapi" || !r.FetchedAt.Equal(agoraBTC) {
		t.Fatalf("1ª: %+v, %v", r, err)
	}

	agora = agoraBTC.Add(ttlTeste - time.Second)
	if _, err := s.GetQuotes(ctx); err != nil || prov.chamadas.Load() != 1 {
		t.Fatalf("dentro do TTL: chamadas = %d, err = %v", prov.chamadas.Load(), err)
	}

	prov.err = errors.New("429")
	agora = agoraBTC.Add(ttlTeste)
	r, err = s.GetQuotes(ctx)
	if err != nil || !r.Stale || !r.FetchedAt.Equal(agoraBTC) || prov.chamadas.Load() != 2 {
		t.Fatalf("falha com cache: %+v, %v, chamadas %d", r, err, prov.chamadas.Load())
	}

	s2 := NewCurrencyQuoteService(&fakeMoedas{err: errors.New("timeout")}, "awesomeapi", ttlTeste, time.Second)
	if _, err := s2.GetQuotes(ctx); !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("sem cache: erro = %v, queria ErrIndisponivel", err)
	}
}
