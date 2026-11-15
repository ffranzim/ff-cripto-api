package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

type handlers struct {
	log *slog.Logger

	bitcoin          BitcoinQuoteService
	bitcoinFontes    BitcoinProviderQuoteService
	bitcoinHistorico BitcoinHistoryService
	moedas           CurrencyQuoteService
	moedasHistorico  CurrencyHistoryService
}

// GET /quotes/bitcoin
//
// Não aceita forceRefresh: a rota é pública e cada refresh gasta a cota das APIs
// externas. O cache (TTL) e o fallback ficam no service.
func (h *handlers) cotacaoBitcoin(w http.ResponseWriter, r *http.Request) {
	res, err := h.bitcoin.GetQuote(r.Context(), false)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCotacaoBitcoinDTO(res))
}

// GET /quotes/bitcoin/{fonte}   fonte = coingecko | binance | awesomeapi
//
// Consulta só aquele provedor, sem fallback e sem gravar histórico.
// Fonte desconhecida -> 404; provedor fora do ar -> 503. "stale" é sempre false.
func (h *handlers) cotacaoBitcoinDaFonte(w http.ResponseWriter, r *http.Request) {
	q, err := h.bitcoinFontes.GetQuoteFrom(r.Context(), chi.URLParam(r, "fonte"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCotacaoBitcoinDTO(domain.BitcoinQuoteResult{Quote: q}))
}

// GET /quotes/bitcoin/history?currency=usd|brl&days=30
//
// CoinGecko market_chart, salvo em historico_bitcoin. currency é obrigatória;
// days vai de 1 a 360 (padrão 30): até 90 dias de hora em hora, acima um ponto
// por dia (campo interval).
func (h *handlers) historicoBitcoin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	moeda := q.Get("currency")
	if moeda == "" {
		writeError(w, http.StatusBadRequest, "entrada_invalida", "currency é obrigatória (usd ou brl)")
		return
	}

	dias := 0
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "entrada_invalida", "days deve ser um inteiro")
			return
		}
		if n == 0 { // 0 no service vale como "padrão"; aqui é entrada inválida
			n = -1
		}
		dias = n
	}

	hist, err := h.bitcoinHistorico.GetHistory(r.Context(), moeda, dias)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toHistoricoBitcoinDTO(hist))
}

// GET /quotes/currencies
//
// Dólar, euro e libra em real (AwesomeAPI), com cache em memória pelo
// BTC_QUOTE_TTL_SECONDS. Falhou: últimas cotações boas com stale=true; sem elas, 503.
func (h *handlers) cotacaoMoedas(w http.ResponseWriter, r *http.Request) {
	res, err := h.moedas.GetQuotes(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCotacaoMoedasDTO(res))
}

// GET /quotes/currencies/history?pair=USD-BRL&days=30
//
// Cotação diária (AwesomeAPI json/daily) em ordem cronológica; o último ponto é
// a cotação de agora. pair: USD-BRL | EUR-BRL | GBP-BRL; days: últimas N cotações
// (dias úteis), de 1 a 30 (padrão 30).
func (h *handlers) historicoMoeda(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	par := q.Get("pair")
	if par == "" {
		writeError(w, http.StatusBadRequest, "entrada_invalida", "pair é obrigatório (ex.: USD-BRL)")
		return
	}
	dias := 0
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n == 0 {
			writeError(w, http.StatusBadRequest, "entrada_invalida", "days deve ser um inteiro de 1 a 30")
			return
		}
		dias = n
	}

	hist, err := h.moedasHistorico.GetHistory(r.Context(), par, dias)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toHistoricoMoedaDTO(hist))
}
