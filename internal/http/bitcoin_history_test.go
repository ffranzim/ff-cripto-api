package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

type stubHistorico struct {
	res   domain.BitcoinHistory
	err   error
	moeda *string
	dias  *int
}

func (s stubHistorico) GetHistory(_ context.Context, moeda string, dias int) (domain.BitcoinHistory, error) {
	if s.moeda != nil {
		*s.moeda, *s.dias = moeda, dias
	}
	return s.res, s.err
}

func TestHistoricoBitcoin(t *testing.T) {
	mc := "1325678901234.56"
	hist := domain.BitcoinHistory{
		Currency: "brl", Days: 30,
		UpdatedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		Points: []domain.BitcoinHistoryPoint{
			{At: time.Date(2026, 11, 8, 11, 0, 0, 0, time.UTC), Price: "360123.45", MarketCap: &mc},
		},
	}

	tests := []struct {
		name       string
		url        string
		stub       stubHistorico
		wantStatus int
		wantMoeda  string
		wantDias   int
		wantBody   []string
	}{
		{"sucesso", "/quotes/bitcoin/history?currency=brl", stubHistorico{res: hist}, 200, "brl", 0,
			[]string{`"currency":"brl"`, `"source":"coingecko"`, `"days":30`, `"updated_at":"2026-11-08T12:00:00Z"`, `"stale":false`,
				`"timestamp":"2026-11-08T11:00:00Z"`, `"price":360123.45`, `"market_cap":1325678901234.56`, `"total_volume":null`}},
		{"repassa days", "/quotes/bitcoin/history?currency=usd&days=7", stubHistorico{res: hist}, 200, "usd", 7, nil},
		{"days=0 é inválido", "/quotes/bitcoin/history?currency=usd&days=0", stubHistorico{err: fmt.Errorf("%w: days", domain.ErrEntradaInvalida)}, 400, "usd", -1, nil},
		{"sem currency", "/quotes/bitcoin/history", stubHistorico{}, 400, "", 0, []string{"currency"}},
		{"days não inteiro", "/quotes/bitcoin/history?currency=usd&days=abc", stubHistorico{}, 400, "", 0, []string{"days"}},
		{"moeda inválida", "/quotes/bitcoin/history?currency=eur", stubHistorico{err: fmt.Errorf("%w: currency", domain.ErrEntradaInvalida)}, 400, "eur", 0, nil},
		{"coingecko fora e sem dados", "/quotes/bitcoin/history?currency=usd", stubHistorico{err: fmt.Errorf("%w: coingecko: 429", domain.ErrIndisponivel)}, 503, "usd", 0, []string{`"indisponivel"`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var moeda string
			dias := -99
			tt.stub.moeda, tt.stub.dias = &moeda, &dias
			router := NewRouter(Deps{
				Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
				DB:               okPinger{},
				BitcoinFontes:    stubFonteQueFalha{}, // a rota fixa tem prioridade sobre /{fonte}
				BitcoinHistorico: tt.stub,
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantMoeda != "" && (moeda != tt.wantMoeda || dias != tt.wantDias) {
				t.Errorf("service recebeu %q/%d, queria %q/%d", moeda, dias, tt.wantMoeda, tt.wantDias)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("corpo %s não contém %s", rec.Body, want)
				}
			}
			if strings.Contains(rec.Body.String(), "429") {
				t.Errorf("corpo expõe detalhe interno: %s", rec.Body)
			}
		})
	}
}

type stubFonteQueFalha struct{}

func (stubFonteQueFalha) GetQuoteFrom(context.Context, string) (domain.BitcoinQuote, error) {
	return domain.BitcoinQuote{}, fmt.Errorf("%w: não deveria ser chamado", domain.ErrNaoEncontrado)
}
