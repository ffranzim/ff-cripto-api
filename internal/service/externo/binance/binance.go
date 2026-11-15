// Package binance é o cliente da API pública da Binance (cotação do Bitcoin).
package binance

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

const (
	urlBinance = "https://api.binance.com/api/v3/ticker/price"

	// A Binance não tem par BTC/USD à vista: BTCUSDT (Tether) é usado como
	// aproximação do dólar. O USDT costuma ficar a menos de 0,1% do USD, mas
	// pode descolar em momentos de estresse do mercado.
	simboloUSD = "BTCUSDT"
	simboloBRL = "BTCBRL"
)

// Binance é o 1º fallback (sem chave).
type Binance struct {
	url    string
	client *http.Client
	agora  func() time.Time
}

func NewBinance(client *http.Client) *Binance {
	return &Binance{url: urlBinance, client: client, agora: time.Now}
}

func (*Binance) Name() string { return domain.FonteBinance }

// [ { "symbol": "BTCUSDT", "price": "string" }, { "symbol": "BTCBRL", "price": "string" } ]
type tickerBinance struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

func (b *Binance) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	// url.Values faz o URL-encode de ["BTCUSDT","BTCBRL"] (colchetes, aspas e vírgula).
	q := url.Values{"symbols": {`["` + simboloUSD + `","` + simboloBRL + `"]`}}

	var tickers []tickerBinance
	if err := httpjson.Get(ctx, b.client, b.url+"?"+q.Encode(), nil, &tickers); err != nil {
		return domain.BitcoinQuote{}, err
	}

	precos := make(map[string]string, len(tickers))
	for _, t := range tickers {
		precos[t.Symbol] = t.Price
	}

	usd, err := httpjson.PrecoPositivo(simboloUSD, precos[simboloUSD])
	if err != nil {
		return domain.BitcoinQuote{}, err
	}
	brl, err := httpjson.PrecoPositivo(simboloBRL, precos[simboloBRL])
	if err != nil {
		return domain.BitcoinQuote{}, err
	}
	return domain.BitcoinQuote{USD: usd, BRL: brl, Source: domain.FonteBinance, FetchedAt: b.agora()}, nil
}
