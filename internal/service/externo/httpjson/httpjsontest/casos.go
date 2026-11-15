package httpjsontest

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// AgoraTeste é o relógio fixo dos provedores nos testes.
var AgoraTeste = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// BitcoinFetcher é o que cada cliente de cotação do Bitcoin expõe.
type BitcoinFetcher interface {
	Fetch(ctx context.Context) (domain.BitcoinQuote, error)
}

// Fabrica monta o provedor apontando para o httptest e com o relógio AgoraTeste.
type Fabrica func(srv *httptest.Server) BitcoinFetcher

type Caso struct {
	Name    string
	Status  int
	Corpo   string
	WantErr error // nil = sucesso
	WantUSD string
	WantBRL string
}

// RodarCasos roda cada caso contra um servidor falso e confere erro ou cotação.
func RodarCasos(t *testing.T, novo Fabrica, fonte string, casos []Caso) {
	t.Helper()
	for _, tt := range casos {
		t.Run(tt.Name, func(t *testing.T) {
			q, err := novo(Servidor(t, tt.Status, tt.Corpo, nil)).Fetch(context.Background())

			if tt.WantErr != nil {
				if !errors.Is(err, tt.WantErr) {
					t.Fatalf("erro = %v, queria %v", err, tt.WantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			want := domain.BitcoinQuote{USD: tt.WantUSD, BRL: tt.WantBRL, Source: fonte, FetchedAt: AgoraTeste}
			if q != want {
				t.Errorf("cotação = %+v, queria %+v", q, want)
			}
		})
	}
}
