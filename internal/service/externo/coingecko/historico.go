package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

const urlCoinGeckoHistorico = "https://api.coingecko.com/api/v3/coins/bitcoin/market_chart"

// CoinGeckoHistorico busca o histórico de preço do Bitcoin (market_chart).
// Com days entre 2 e 90 a CoinGecko devolve um ponto por hora; acima de 90,
// um por dia (00:00 UTC) mais o instante atual.
type CoinGeckoHistorico struct {
	url    string
	apiKey string
	client *http.Client
}

// NewCoinGeckoHistorico: apiKey vazia ainda funciona (API pública), com limite menor.
func NewCoinGeckoHistorico(apiKey string, client *http.Client) *CoinGeckoHistorico {
	return &CoinGeckoHistorico{url: urlCoinGeckoHistorico, apiKey: apiKey, client: client}
}

// { "prices": [[ms, n], ...], "market_caps": [[ms, n], ...], "total_volumes": [[ms, n], ...] }
type respostaMarketChart struct {
	Prices       [][]json.Number `json:"prices"`
	MarketCaps   [][]json.Number `json:"market_caps"`
	TotalVolumes [][]json.Number `json:"total_volumes"`
}

// FetchHistory devolve os pontos dos últimos "dias" em "moeda" (usd | brl),
// em ordem cronológica e sem instantes repetidos.
func (c *CoinGeckoHistorico) FetchHistory(ctx context.Context, moeda string, dias int) ([]domain.BitcoinHistoryPoint, error) {
	q := url.Values{"vs_currency": {moeda}, "days": {strconv.Itoa(dias)}}
	var headers map[string]string
	if c.apiKey != "" {
		headers = map[string]string{"x-cg-demo-api-key": c.apiKey}
	}

	var body respostaMarketChart
	if err := httpjson.Get(ctx, c.client, c.url+"?"+q.Encode(), headers, &body); err != nil {
		return nil, err
	}
	if len(body.Prices) == 0 {
		return nil, fmt.Errorf("%w: prices", httpjson.ErrCampoAusente)
	}

	// market_caps e total_volumes são casados com prices pelo instante, não pelo
	// índice: se a CoinGecko omitir um ponto, o valor fica nil em vez de deslocar.
	caps, err := valoresPorInstante("market_caps", body.MarketCaps)
	if err != nil {
		return nil, err
	}
	vols, err := valoresPorInstante("total_volumes", body.TotalVolumes)
	if err != nil {
		return nil, err
	}

	pontos := make([]domain.BitcoinHistoryPoint, 0, len(body.Prices))
	for i, par := range body.Prices {
		ms, valor, err := parDoGrafico(fmt.Sprintf("prices[%d]", i), par)
		if err != nil {
			return nil, err
		}
		preco, err := httpjson.PrecoPositivo(fmt.Sprintf("prices[%d]", i), valor)
		if err != nil {
			return nil, err
		}
		p := domain.BitcoinHistoryPoint{At: time.UnixMilli(ms).UTC(), Price: preco}
		if v, ok := caps[ms]; ok {
			p.MarketCap = &v
		}
		if v, ok := vols[ms]; ok {
			p.TotalVolume = &v
		}

		// Instante repetido: fica o último (a chave do banco é moeda + momento).
		if n := len(pontos); n > 0 && !p.At.After(pontos[n-1].At) {
			if p.At.Equal(pontos[n-1].At) {
				pontos[n-1] = p
				continue
			}
			return nil, fmt.Errorf("%w: prices fora de ordem em %d", httpjson.ErrJSONInvalido, i)
		}
		pontos = append(pontos, p)
	}
	return pontos, nil
}

func valoresPorInstante(campo string, pares [][]json.Number) (map[int64]string, error) {
	m := make(map[int64]string, len(pares))
	for i, par := range pares {
		nome := fmt.Sprintf("%s[%d]", campo, i)
		ms, valor, err := parDoGrafico(nome, par)
		if err != nil {
			return nil, err
		}
		if valor == "" { // [ms, null]: sem valor naquele instante
			continue
		}
		v, err := httpjson.ValorNaoNegativo(nome, valor)
		if err != nil {
			return nil, err
		}
		m[ms] = v
	}
	return m, nil
}

// parDoGrafico lê [timestamp_ms, valor].
func parDoGrafico(campo string, par []json.Number) (int64, string, error) {
	if len(par) != 2 {
		return 0, "", fmt.Errorf("%w: %s deveria ter 2 itens, tem %d", httpjson.ErrJSONInvalido, campo, len(par))
	}
	ms, err := par[0].Int64()
	if err != nil {
		// Alguns endpoints mandam o timestamp como 1.7e12.
		f, errF := par[0].Float64()
		if errF != nil || f <= 0 {
			return 0, "", fmt.Errorf("%w: %s timestamp %q", httpjson.ErrJSONInvalido, campo, par[0])
		}
		ms = int64(f)
	}
	if ms <= 0 {
		return 0, "", fmt.Errorf("%w: %s timestamp %d", httpjson.ErrJSONInvalido, campo, ms)
	}
	return ms, par[1].String(), nil
}
