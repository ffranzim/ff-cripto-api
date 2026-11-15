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

type stubMoedas struct {
	res domain.CurrencyQuotesResult
	err error
}

func (s stubMoedas) GetQuotes(context.Context) (domain.CurrencyQuotesResult, error) {
	return s.res, s.err
}

func TestCotacaoMoedas(t *testing.T) {
	res := domain.CurrencyQuotesResult{
		Source: "awesomeapi", FetchedAt: time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC),
		Quotes: []domain.CurrencyQuote{{
			Code: "USD", CodeIn: "BRL", Name: "Dólar Americano/Real Brasileiro",
			Bid: "5.5085", Ask: "5.5091", High: "5.5275", Low: "5.4898", VarBid: "-0.0035", PctChange: "-0.063576",
			QuotedAt: time.Date(2026, 11, 9, 19, 43, 54, 0, time.UTC),
		}},
	}
	tests := []struct {
		name       string
		stub       stubMoedas
		wantStatus int
		wantBody   []string
	}{
		{"sucesso", stubMoedas{res: res}, 200, []string{`"source":"awesomeapi"`, `"stale":false`, `"code":"USD"`, `"codein":"BRL"`,
			`"bid":5.5085`, `"ask":5.5091`, `"var_bid":-0.0035`, `"pct_change":-0.063576`, `"quoted_at":"2026-11-09T19:43:54Z"`, `"fetched_at":"2026-11-09T22:00:00Z"`}},
		{"indisponível", stubMoedas{err: fmt.Errorf("%w: awesomeapi: 429", domain.ErrIndisponivel)}, 503, []string{`"indisponivel"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Deps{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: okPinger{}, Moedas: tt.stub,
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/quotes/currencies", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, queria %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			for _, w := range tt.wantBody {
				if !strings.Contains(rec.Body.String(), w) {
					t.Errorf("corpo %s não contém %s", rec.Body, w)
				}
			}
			if strings.Contains(rec.Body.String(), "429") {
				t.Errorf("vaza detalhe interno: %s", rec.Body)
			}
		})
	}
}

type stubHistMoeda struct {
	res  domain.CurrencyHistory
	err  error
	par  *string
	dias *int
}

func (s stubHistMoeda) GetHistory(_ context.Context, par string, dias int) (domain.CurrencyHistory, error) {
	*s.par, *s.dias = par, dias
	return s.res, s.err
}

func TestHistoricoMoeda(t *testing.T) {
	hist := domain.CurrencyHistory{
		Pair: "USD-BRL", Days: 7, Source: "awesomeapi", UpdatedAt: time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC),
		Points: []domain.CurrencyQuote{{
			Code: "USD", CodeIn: "BRL", Name: "Dólar Americano/Real Brasileiro",
			Bid: "5.503", Ask: "5.5035", High: "5.5235", Low: "5.4835", VarBid: "-0.0008", PctChange: "-0.014534",
			QuotedAt: time.Date(2026, 11, 6, 20, 4, 49, 0, time.UTC),
		}},
	}
	tests := []struct {
		name, url  string
		err        error
		wantStatus int
		wantPar    string
		wantDias   int
		wantBody   []string
	}{
		{"sucesso", "/quotes/currencies/history?pair=USD-BRL&days=7", nil, 200, "USD-BRL", 7,
			[]string{`"pair":"USD-BRL"`, `"code":"USD"`, `"codein":"BRL"`, `"name":"Dólar Americano/Real Brasileiro"`, `"days":7`,
				`"stale":false`, `"timestamp":"2026-11-06T20:04:49Z"`, `"bid":5.503`, `"var_bid":-0.0008`, `"pct_change":-0.014534`}},
		{"days padrão", "/quotes/currencies/history?pair=EUR-BRL", nil, 200, "EUR-BRL", 0, nil},
		{"sem pair", "/quotes/currencies/history", nil, 400, "", 0, []string{"pair"}},
		{"days=0", "/quotes/currencies/history?pair=USD-BRL&days=0", nil, 400, "", 0, []string{"days"}},
		{"days texto", "/quotes/currencies/history?pair=USD-BRL&days=x", nil, 400, "", 0, []string{"days"}},
		{"par inválido", "/quotes/currencies/history?pair=BTC-BRL", fmt.Errorf("%w: pair", domain.ErrEntradaInvalida), 400, "BTC-BRL", 0, nil},
		{"indisponível", "/quotes/currencies/history?pair=USD-BRL", fmt.Errorf("%w: 429", domain.ErrIndisponivel), 503, "USD-BRL", 0, []string{`"indisponivel"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var par string
			dias := -99
			router := NewRouter(Deps{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: okPinger{},
				MoedasHistorico: stubHistMoeda{res: hist, err: tt.err, par: &par, dias: &dias},
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, queria %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantPar != "" && (par != tt.wantPar || dias != tt.wantDias) {
				t.Errorf("service recebeu %q/%d, queria %q/%d", par, dias, tt.wantPar, tt.wantDias)
			}
			for _, w := range tt.wantBody {
				if !strings.Contains(rec.Body.String(), w) {
					t.Errorf("corpo %s não contém %s", rec.Body, w)
				}
			}
			if strings.Contains(rec.Body.String(), "429") {
				t.Errorf("vaza detalhe interno: %s", rec.Body)
			}
		})
	}
}
