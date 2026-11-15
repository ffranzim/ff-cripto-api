package httpapi

import (
	"context"
	"errors"
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

type stubBitcoin struct {
	res domain.BitcoinQuoteResult
	err error
}

func (s stubBitcoin) GetQuote(context.Context, bool) (domain.BitcoinQuoteResult, error) {
	return s.res, s.err
}

func TestCotacaoBitcoin(t *testing.T) {
	q := domain.BitcoinQuote{
		ID: 1, USD: "67012.34", BRL: "360123.00000000", Source: "binance",
		FetchedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name       string
		stub       stubBitcoin
		wantStatus int
		wantBody   []string
	}{
		{"cotação fresca", stubBitcoin{res: domain.BitcoinQuoteResult{Quote: q}}, 200,
			[]string{`"usd":67012.34`, `"brl":360123.00000000`, `"source":"binance"`, `"fetched_at":"2026-11-08T12:00:00Z"`, `"stale":false`}},
		{"cotação vencida", stubBitcoin{res: domain.BitcoinQuoteResult{Quote: q, Stale: true}}, 200,
			[]string{`"stale":true`}},
		{"todos os provedores falharam e sem cache", stubBitcoin{err: fmt.Errorf("%w: coingecko: timeout", domain.ErrIndisponivel)}, 503,
			[]string{`"indisponivel"`}},
		{"erro do banco", stubBitcoin{err: errors.New("banco fora")}, 500,
			[]string{`"erro_interno"`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Deps{
				Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
				DB:      okPinger{},
				Bitcoin: tt.stub,
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/quotes/bitcoin", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("corpo %s não contém %s", rec.Body, want)
				}
			}
			// O motivo interno (provedor/timeout) não vaza para o cliente.
			if strings.Contains(rec.Body.String(), "coingecko") {
				t.Errorf("corpo expõe detalhe interno: %s", rec.Body)
			}
		})
	}
}

func TestCotacaoBitcoinDesligada(t *testing.T) {
	router := NewRouter(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: okPinger{}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/quotes/bitcoin", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, queria 404", rec.Code)
	}
}

type stubBitcoinFontes struct {
	q   domain.BitcoinQuote
	err error
	got string
}

func (s *stubBitcoinFontes) GetQuoteFrom(_ context.Context, fonte string) (domain.BitcoinQuote, error) {
	s.got = fonte
	return s.q, s.err
}

func TestCotacaoBitcoinDaFonte(t *testing.T) {
	q := domain.BitcoinQuote{
		USD: "67012.34", BRL: "360123.5", Source: "awesomeapi",
		FetchedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name       string
		stub       *stubBitcoinFontes
		wantStatus int
		wantBody   []string
	}{
		{"sucesso", &stubBitcoinFontes{q: q}, 200,
			[]string{`"usd":67012.34`, `"brl":360123.5`, `"source":"awesomeapi"`, `"stale":false`}},
		{"fonte desconhecida", &stubBitcoinFontes{err: fmt.Errorf("%w: provedor", domain.ErrNaoEncontrado)}, 404,
			[]string{`"nao_encontrado"`}},
		{"provedor fora do ar", &stubBitcoinFontes{err: fmt.Errorf("%w: awesomeapi: status 429", domain.ErrIndisponivel)}, 503,
			[]string{`"indisponivel"`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Deps{
				Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
				DB:            okPinger{},
				BitcoinFontes: tt.stub,
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/quotes/bitcoin/awesomeapi", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.stub.got != "awesomeapi" {
				t.Errorf("fonte repassada = %q", tt.stub.got)
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
