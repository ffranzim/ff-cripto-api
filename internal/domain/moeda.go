package domain

import "time"

// CurrencyQuote é a cotação de uma moeda em outra (ex.: USD em BRL), da AwesomeAPI.
// Valores em texto decimal exato. VarBid e PctChange podem ser negativos.
type CurrencyQuote struct {
	Code      string // "USD"
	CodeIn    string // "BRL"
	Name      string // "Dólar Americano/Real Brasileiro"
	Bid       string
	Ask       string
	High      string
	Low       string
	VarBid    string
	PctChange string
	QuotedAt  time.Time // instante da cotação informado pela AwesomeAPI
}

// CurrencyHistory é a cotação diária de um par (ex.: "USD-BRL"), em ordem
// cronológica. O último ponto é a cotação de agora. Stale = a busca falhou e
// são os últimos pontos bons.
type CurrencyHistory struct {
	Pair      string
	Days      int
	Points    []CurrencyQuote
	Source    string
	UpdatedAt time.Time
	Stale     bool
}

// CurrencyQuotesResult: Stale = a busca falhou e são as últimas cotações boas.
type CurrencyQuotesResult struct {
	Quotes    []CurrencyQuote
	Source    string
	FetchedAt time.Time
	Stale     bool
}
