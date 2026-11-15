package interno

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

type fakeMoedasDiario struct {
	err      error
	chamadas atomic.Int32
}

// 30 cotações diárias terminando em agoraBTC, em ordem cronológica.
func (f *fakeMoedasDiario) FetchDaily(_ context.Context, par string, dias int) ([]domain.CurrencyQuote, error) {
	f.chamadas.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	pts := make([]domain.CurrencyQuote, dias)
	for i := range pts {
		pts[i] = domain.CurrencyQuote{Code: par[:3], CodeIn: "BRL", Bid: "5.5", QuotedAt: agoraBTC.Add(-time.Duration(dias-1-i) * 24 * time.Hour)}
	}
	return pts, nil
}

func novoHistMoedas(prov *fakeMoedasDiario, agora *time.Time) *CurrencyHistoryService {
	s := NewCurrencyHistoryService(prov, []string{"USD-BRL", "EUR-BRL"}, "awesomeapi", time.Hour, time.Second)
	s.agora = func() time.Time { return *agora }
	return s
}

func TestHistoricoMoedas(t *testing.T) {
	prov := &fakeMoedasDiario{}
	agora := agoraBTC
	s := novoHistMoedas(prov, &agora)
	ctx := context.Background()

	h, err := s.GetHistory(ctx, "USD-BRL", 0)
	if err != nil || len(h.Points) != 30 || h.Days != 30 || h.Pair != "USD-BRL" || h.Stale || !h.UpdatedAt.Equal(agoraBTC) {
		t.Fatalf("padrão: %d pontos, %+v, %v", len(h.Points), h.Days, err)
	}

	// Recorte: últimas 7, vindo do cache (não altera o cache).
	h, err = s.GetHistory(ctx, "USD-BRL", 7)
	if err != nil || len(h.Points) != 7 || !h.Points[6].QuotedAt.Equal(agoraBTC) || prov.chamadas.Load() != 1 {
		t.Fatalf("7: %d pontos, chamadas %d, %v", len(h.Points), prov.chamadas.Load(), err)
	}
	if h, _ = s.GetHistory(ctx, "USD-BRL", 30); len(h.Points) != 30 {
		t.Fatalf("cache foi recortado: %d", len(h.Points))
	}

	// Cache por par.
	if _, err := s.GetHistory(ctx, "EUR-BRL", 0); err != nil || prov.chamadas.Load() != 2 {
		t.Fatalf("EUR: chamadas %d, %v", prov.chamadas.Load(), err)
	}

	// TTL vencido + falha -> stale.
	prov.err = errors.New("429")
	agora = agoraBTC.Add(time.Hour)
	h, err = s.GetHistory(ctx, "USD-BRL", 0)
	if err != nil || !h.Stale || len(h.Points) != 30 || prov.chamadas.Load() != 3 {
		t.Fatalf("stale: %+v, %v", h.Stale, err)
	}
}

func TestHistoricoMoedasErros(t *testing.T) {
	agora := agoraBTC
	prov := &fakeMoedasDiario{}
	s := novoHistMoedas(prov, &agora)
	for _, c := range []struct {
		par  string
		dias int
	}{{"BTC-BRL", 0}, {"usd-brl", 0}, {"", 0}, {"USD-BRL", -1}, {"USD-BRL", 31}} {
		if _, err := s.GetHistory(context.Background(), c.par, c.dias); !errors.Is(err, domain.ErrEntradaInvalida) {
			t.Errorf("%q/%d: erro = %v", c.par, c.dias, err)
		}
	}
	if prov.chamadas.Load() != 0 {
		t.Errorf("entrada inválida chamou a API")
	}

	s = novoHistMoedas(&fakeMoedasDiario{err: errors.New("timeout")}, &agora)
	if _, err := s.GetHistory(context.Background(), "USD-BRL", 0); !errors.Is(err, domain.ErrIndisponivel) {
		t.Fatalf("sem cache: erro = %v", err)
	}
}
