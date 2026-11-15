// Package awesomeapi é o cliente da AwesomeAPI (economia.awesomeapi.com.br):
// cotação do Bitcoin e de moedas fiduciárias.
package awesomeapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

const urlAwesomeAPI = "https://economia.awesomeapi.com.br/json/last/BTC-USD,BTC-BRL"

// AwesomeAPI é o 2º fallback. Sem chave a cota é por IP e acaba rápido em IP de
// datacenter (429 QuotaExceeded); com a chave gratuita são 100 mil req/mês.
type AwesomeAPI struct {
	url    string
	apiKey string
	client *http.Client
	agora  func() time.Time
}

// NewAwesomeAPI: apiKey vazia ainda funciona (API pública), com cota por IP.
func NewAwesomeAPI(apiKey string, client *http.Client) *AwesomeAPI {
	return &AwesomeAPI{url: urlAwesomeAPI, apiKey: apiKey, client: client, agora: time.Now}
}

func (*AwesomeAPI) Name() string { return domain.FonteAwesomeAPI }

// { "BTCUSD": { "bid": "string" }, "BTCBRL": { "bid": "string" } }
type cotacaoAwesome struct {
	Bid string `json:"bid"`
}

func (a *AwesomeAPI) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	var headers map[string]string
	if a.apiKey != "" {
		headers = map[string]string{"x-api-key": a.apiKey}
	}

	var body map[string]cotacaoAwesome
	if err := httpjson.Get(ctx, a.client, a.url, headers, &body); err != nil {
		return domain.BitcoinQuote{}, err
	}

	usdPar, ok := body["BTCUSD"]
	if !ok {
		return domain.BitcoinQuote{}, fmt.Errorf("%w: BTCUSD", httpjson.ErrCampoAusente)
	}
	brlPar, ok := body["BTCBRL"]
	if !ok {
		return domain.BitcoinQuote{}, fmt.Errorf("%w: BTCBRL", httpjson.ErrCampoAusente)
	}

	usd, err := httpjson.PrecoPositivo("BTCUSD.bid", usdPar.Bid)
	if err != nil {
		return domain.BitcoinQuote{}, err
	}
	brl, err := httpjson.PrecoPositivo("BTCBRL.bid", brlPar.Bid)
	if err != nil {
		return domain.BitcoinQuote{}, err
	}

	return domain.BitcoinQuote{USD: usd, BRL: brl, Source: domain.FonteAwesomeAPI, FetchedAt: a.agora()}, nil
}
