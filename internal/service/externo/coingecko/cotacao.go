// Package coingecko é o cliente da CoinGecko: cotação e histórico do Bitcoin.
package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

const urlCoinGecko = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,brl"

// CoinGecko é o provedor principal. Com a chave do plano Demo
// (header x-cg-demo-api-key) o limite de requisições é maior.
type CoinGecko struct {
	url    string
	apiKey string
	client *http.Client
	agora  func() time.Time // trocado nos testes
}

// NewCoinGecko: apiKey vazia ainda funciona (API pública), com limite menor.
func NewCoinGecko(apiKey string, client *http.Client) *CoinGecko {
	return &CoinGecko{url: urlCoinGecko, apiKey: apiKey, client: client, agora: time.Now}
}

func (*CoinGecko) Name() string { return domain.FonteCoinGecko }

// { "bitcoin": { "usd": number, "brl": number } }
type respostaCoinGecko struct {
	Bitcoin *struct {
		USD json.Number `json:"usd"`
		BRL json.Number `json:"brl"`
	} `json:"bitcoin"`
}

func (c *CoinGecko) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	var headers map[string]string
	if c.apiKey != "" {
		headers = map[string]string{"x-cg-demo-api-key": c.apiKey}
	}

	var body respostaCoinGecko
	if err := httpjson.Get(ctx, c.client, c.url, headers, &body); err != nil {
		return domain.BitcoinQuote{}, err
	}
	if body.Bitcoin == nil {
		return domain.BitcoinQuote{}, fmt.Errorf("%w: bitcoin", httpjson.ErrCampoAusente)
	}

	usd, err := httpjson.PrecoPositivo("bitcoin.usd", body.Bitcoin.USD.String())
	if err != nil {
		return domain.BitcoinQuote{}, err
	}
	brl, err := httpjson.PrecoPositivo("bitcoin.brl", body.Bitcoin.BRL.String())
	if err != nil {
		return domain.BitcoinQuote{}, err
	}

	return domain.BitcoinQuote{USD: usd, BRL: brl, Source: domain.FonteCoinGecko, FetchedAt: c.agora()}, nil
}
