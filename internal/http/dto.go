package httpapi

import (
	"encoding/json"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// DTOs = contrato JSON com o app Flutter. As tags `json:"..."` fazem o papel do
// @JsonKey do json_serializable. Campos snake_case; `omitempty` não é usado
// para que o app sempre receba a chave (com null quando não houver valor).

const formatoData = "2006-01-02" // layout de data do Go: sempre esta data de referência

// cotacaoBitcoinDTO: preços como número JSON exato (json.Number, sem float64).
// stale = true quando todos os provedores falharam e a cotação está vencida.
type cotacaoBitcoinDTO struct {
	USD       json.Number `json:"usd"`
	BRL       json.Number `json:"brl"`
	Source    string      `json:"source"`
	FetchedAt time.Time   `json:"fetched_at"`
	Stale     bool        `json:"stale"`
}

func toCotacaoBitcoinDTO(r domain.BitcoinQuoteResult) cotacaoBitcoinDTO {
	return cotacaoBitcoinDTO{
		USD:       json.Number(r.Quote.USD),
		BRL:       json.Number(r.Quote.BRL),
		Source:    r.Quote.Source,
		FetchedAt: r.Quote.FetchedAt,
		Stale:     r.Stale,
	}
}

// historicoBitcoinDTO: o gráfico de uma moeda. updated_at = última sincronização
// com a CoinGecko; stale = true quando ela falhou e os pontos estão vencidos.
type historicoBitcoinDTO struct {
	Currency  string                     `json:"currency"`
	Source    string                     `json:"source"`
	Days      int                        `json:"days"`
	Interval  string                     `json:"interval"` // hourly | daily
	UpdatedAt time.Time                  `json:"updated_at"`
	Stale     bool                       `json:"stale"`
	Prices    []pontoHistoricoBitcoinDTO `json:"prices"`
}

type pontoHistoricoBitcoinDTO struct {
	Timestamp   time.Time    `json:"timestamp"`
	Price       json.Number  `json:"price"`
	MarketCap   *json.Number `json:"market_cap"`
	TotalVolume *json.Number `json:"total_volume"`
}

func toHistoricoBitcoinDTO(h domain.BitcoinHistory) historicoBitcoinDTO {
	pontos := make([]pontoHistoricoBitcoinDTO, 0, len(h.Points))
	for _, p := range h.Points {
		pontos = append(pontos, pontoHistoricoBitcoinDTO{
			Timestamp:   p.At,
			Price:       json.Number(p.Price),
			MarketCap:   numeroPtr(p.MarketCap),
			TotalVolume: numeroPtr(p.TotalVolume),
		})
	}
	return historicoBitcoinDTO{
		Currency:  h.Currency,
		Source:    domain.FonteCoinGecko,
		Days:      h.Days,
		Interval:  h.Interval,
		UpdatedAt: h.UpdatedAt,
		Stale:     h.Stale,
		Prices:    pontos,
	}
}

func numeroPtr(s *string) *json.Number {
	if s == nil {
		return nil
	}
	n := json.Number(*s)
	return &n
}

// cotacaoMoedasDTO: valores como número JSON exato. var_bid e pct_change podem
// ser negativos; quoted_at é o instante da cotação na AwesomeAPI.
type cotacaoMoedasDTO struct {
	Source    string            `json:"source"`
	FetchedAt time.Time         `json:"fetched_at"`
	Stale     bool              `json:"stale"`
	Dados     []cotacaoMoedaDTO `json:"dados"`
}

type cotacaoMoedaDTO struct {
	Code      string      `json:"code"`
	CodeIn    string      `json:"codein"`
	Name      string      `json:"name"`
	Bid       json.Number `json:"bid"`
	Ask       json.Number `json:"ask"`
	High      json.Number `json:"high"`
	Low       json.Number `json:"low"`
	VarBid    json.Number `json:"var_bid"`
	PctChange json.Number `json:"pct_change"`
	QuotedAt  time.Time   `json:"quoted_at"`
}

func toCotacaoMoedasDTO(r domain.CurrencyQuotesResult) cotacaoMoedasDTO {
	dados := make([]cotacaoMoedaDTO, 0, len(r.Quotes))
	for _, q := range r.Quotes {
		dados = append(dados, cotacaoMoedaDTO{
			Code: q.Code, CodeIn: q.CodeIn, Name: q.Name,
			Bid: json.Number(q.Bid), Ask: json.Number(q.Ask),
			High: json.Number(q.High), Low: json.Number(q.Low),
			VarBid: json.Number(q.VarBid), PctChange: json.Number(q.PctChange),
			QuotedAt: q.QuotedAt,
		})
	}
	return cotacaoMoedasDTO{Source: r.Source, FetchedAt: r.FetchedAt, Stale: r.Stale, Dados: dados}
}

// historicoMoedaDTO: cotação diária de um par, em ordem cronológica.
type historicoMoedaDTO struct {
	Pair      string                   `json:"pair"`
	Code      string                   `json:"code"`
	CodeIn    string                   `json:"codein"`
	Name      string                   `json:"name"`
	Source    string                   `json:"source"`
	Days      int                      `json:"days"`
	UpdatedAt time.Time                `json:"updated_at"`
	Stale     bool                     `json:"stale"`
	Dados     []pontoHistoricoMoedaDTO `json:"dados"`
}

type pontoHistoricoMoedaDTO struct {
	Timestamp time.Time   `json:"timestamp"`
	Bid       json.Number `json:"bid"`
	Ask       json.Number `json:"ask"`
	High      json.Number `json:"high"`
	Low       json.Number `json:"low"`
	VarBid    json.Number `json:"var_bid"`
	PctChange json.Number `json:"pct_change"`
}

func toHistoricoMoedaDTO(h domain.CurrencyHistory) historicoMoedaDTO {
	dto := historicoMoedaDTO{
		Pair: h.Pair, Source: h.Source, Days: h.Days, UpdatedAt: h.UpdatedAt, Stale: h.Stale,
		Dados: make([]pontoHistoricoMoedaDTO, 0, len(h.Points)),
	}
	for _, p := range h.Points {
		dto.Code, dto.CodeIn, dto.Name = p.Code, p.CodeIn, p.Name
		dto.Dados = append(dto.Dados, pontoHistoricoMoedaDTO{
			Timestamp: p.QuotedAt,
			Bid:       json.Number(p.Bid), Ask: json.Number(p.Ask),
			High: json.Number(p.High), Low: json.Number(p.Low),
			VarBid: json.Number(p.VarBid), PctChange: json.Number(p.PctChange),
		})
	}
	return dto
}
