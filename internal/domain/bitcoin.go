package domain

import "time"

// Provedores de cotação do Bitcoin; o valor vai para BitcoinQuote.Source e para
// a variável BTC_QUOTE_PROVIDERS.
const (
	FonteCoinGecko  = "coingecko"
	FonteBinance    = "binance"
	FonteAwesomeAPI = "awesomeapi"
)

// BitcoinQuote é a cotação normalizada, igual para qualquer provedor.
type BitcoinQuote struct {
	ID int64 // 0 enquanto não foi salva
	// USD e BRL ficam como texto decimal exato ("67012.34"), como o
	// Rendimento.ValorPorCota: nada de float64 para dinheiro. No banco são numeric.
	USD       string
	BRL       string
	Source    string // FonteCoinGecko | FonteBinance | FonteAwesomeAPI
	FetchedAt time.Time
}

// BitcoinQuoteResult é o que o service devolve: a cotação e se ela está vencida
// (Stale = todos os provedores falharam e foi usada a última salva, fora do TTL).
type BitcoinQuoteResult struct {
	Quote BitcoinQuote
	Stale bool
}

// Moedas do histórico do Bitcoin (vs_currency da CoinGecko).
const (
	MoedaUSD = "usd"
	MoedaBRL = "brl"
)

// BitcoinHistoryPoint é um ponto do gráfico (market_chart da CoinGecko).
// Valores em texto decimal exato, como o BitcoinQuote. MarketCap e TotalVolume
// são nil quando a CoinGecko não manda o valor daquele instante.
type BitcoinHistoryPoint struct {
	At          time.Time
	Price       string
	MarketCap   *string
	TotalVolume *string
}

// Granularidade dos pontos de BitcoinHistory.
const (
	IntervaloHora = "hourly"
	IntervaloDia  = "daily"
)

// BitcoinHistory é o que o service devolve para uma moeda.
// UpdatedAt = última sincronização com a CoinGecko; Stale = a sincronização
// falhou e os pontos são os salvos, mais velhos que o TTL.
type BitcoinHistory struct {
	Currency  string
	Days      int    // janela pedida
	Interval  string // IntervaloHora | IntervaloDia
	Points    []BitcoinHistoryPoint
	UpdatedAt time.Time
	Stale     bool
}
