// Package httpapi expõe a API REST. O pacote se chama httpapi (e não http)
// para não colidir com net/http dentro dos arquivos.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// BitcoinQuoteService é satisfeito pelo *interno.BitcoinQuoteService.
type BitcoinQuoteService interface {
	GetQuote(ctx context.Context, forceRefresh bool) (domain.BitcoinQuoteResult, error)
}

// BitcoinProviderQuoteService é satisfeito pelo *interno.BitcoinProviderQuoteService.
type BitcoinProviderQuoteService interface {
	GetQuoteFrom(ctx context.Context, fonte string) (domain.BitcoinQuote, error)
}

// BitcoinHistoryService é satisfeito pelo *interno.BitcoinHistoryService.
type BitcoinHistoryService interface {
	GetHistory(ctx context.Context, moeda string, dias int) (domain.BitcoinHistory, error)
}

// CurrencyQuoteService é satisfeito pelo *interno.CurrencyQuoteService.
type CurrencyQuoteService interface {
	GetQuotes(ctx context.Context) (domain.CurrencyQuotesResult, error)
}

// CurrencyHistoryService é satisfeito pelo *interno.CurrencyHistoryService.
type CurrencyHistoryService interface {
	GetHistory(ctx context.Context, par string, dias int) (domain.CurrencyHistory, error)
}

// Pinger é satisfeito pelo *pgxpool.Pool; usado no /healthz.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Logger *slog.Logger
	DB     Pinger

	// Bitcoin: nil = GET /quotes/bitcoin desligada.
	Bitcoin BitcoinQuoteService
	// BitcoinFontes: nil = GET /quotes/bitcoin/{fonte} desligada.
	BitcoinFontes BitcoinProviderQuoteService
	// BitcoinHistorico: nil = GET /quotes/bitcoin/history desligada.
	BitcoinHistorico BitcoinHistoryService
	// Moedas: nil = GET /quotes/currencies desligada.
	Moedas CurrencyQuoteService
	// MoedasHistorico: nil = GET /quotes/currencies/history desligada.
	MoedasHistorico CurrencyHistoryService
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // o Caddy envia X-Forwarded-For
	r.Use(requestLogger(d.Logger))
	r.Use(recoverer(d.Logger))
	r.Use(middleware.Timeout(15 * time.Second)) // cancela o ctx da request

	h := &handlers{log: d.Logger, bitcoin: d.Bitcoin, bitcoinFontes: d.BitcoinFontes, bitcoinHistorico: d.BitcoinHistorico, moedas: d.Moedas, moedasHistorico: d.MoedasHistorico}

	r.Get("/healthz", healthz(d.DB))
	if d.Bitcoin != nil {
		r.Get("/quotes/bitcoin", h.cotacaoBitcoin)
	}
	if d.Moedas != nil {
		r.Get("/quotes/currencies", h.cotacaoMoedas)
	}
	if d.MoedasHistorico != nil {
		r.Get("/quotes/currencies/history", h.historicoMoeda)
	}
	// Rota fixa: o chi a prefere a /quotes/bitcoin/{fonte}.
	if d.BitcoinHistorico != nil {
		r.Get("/quotes/bitcoin/history", h.historicoBitcoin)
	}
	if d.BitcoinFontes != nil {
		r.Get("/quotes/bitcoin/{fonte}", h.cotacaoBitcoinDaFonte)
	}

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "nao_encontrado", "rota não encontrada")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "metodo_nao_permitido", "método não permitido")
	})

	return r
}

func healthz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel() // defer roda ao sair da função (≈ finally do Dart)
		if err := db.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "banco_indisponivel", "banco indisponível")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
