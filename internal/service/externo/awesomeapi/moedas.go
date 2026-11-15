package awesomeapi

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
	"github.com/franzim/ff-cripto-api/internal/service/externo/httpjson"
)

// ParesMoedas: pares buscados na AwesomeAPI, na ordem em que a API os devolve.
var ParesMoedas = []string{"USD-BRL", "EUR-BRL", "GBP-BRL"}

const (
	urlAwesomeAPIMoedas       = "https://economia.awesomeapi.com.br/json/last/"
	urlAwesomeAPIMoedasDiario = "https://economia.awesomeapi.com.br/json/daily/"
)

// AwesomeAPIMoedas busca a cotação de moedas fiduciárias em real.
// Usa a mesma chave (x-api-key) do provedor de Bitcoin.
type AwesomeAPIMoedas struct {
	url       string
	urlDiario string
	pares     []string
	apiKey    string
	client    *http.Client
}

func NewAwesomeAPIMoedas(apiKey string, client *http.Client) *AwesomeAPIMoedas {
	return &AwesomeAPIMoedas{
		url:       urlAwesomeAPIMoedas + strings.Join(ParesMoedas, ","),
		urlDiario: urlAwesomeAPIMoedasDiario,
		pares:     ParesMoedas,
		apiKey:    apiKey,
		client:    client,
	}
}

// { "USDBRL": { "code": "USD", "codein": "BRL", "name": "...", "bid": "5.5085", ..., "timestamp": "1794264234" } }
type moedaAwesome struct {
	Code      string `json:"code"`
	CodeIn    string `json:"codein"`
	Name      string `json:"name"`
	High      string `json:"high"`
	Low       string `json:"low"`
	VarBid    string `json:"varBid"`
	PctChange string `json:"pctChange"`
	Bid       string `json:"bid"`
	Ask       string `json:"ask"`
	Timestamp string `json:"timestamp"`
}

// FetchCurrencies devolve as cotações na ordem de ParesMoedas; falta de um par é erro.
func (a *AwesomeAPIMoedas) FetchCurrencies(ctx context.Context) ([]domain.CurrencyQuote, error) {
	var body map[string]moedaAwesome
	if err := httpjson.Get(ctx, a.client, a.url, a.headers(), &body); err != nil {
		return nil, err
	}

	cotacoes := make([]domain.CurrencyQuote, 0, len(a.pares))
	for _, par := range a.pares {
		chave := strings.ReplaceAll(par, "-", "") // "USD-BRL" -> "USDBRL"
		m, ok := body[chave]
		if !ok {
			return nil, fmt.Errorf("%w: %s", httpjson.ErrCampoAusente, chave)
		}
		q, err := cotacaoMoeda(chave, m)
		if err != nil {
			return nil, err
		}
		cotacoes = append(cotacoes, q)
	}
	return cotacoes, nil
}

func (a *AwesomeAPIMoedas) headers() map[string]string {
	if a.apiKey == "" {
		return nil
	}
	return map[string]string{"x-api-key": a.apiKey}
}

// FetchDaily devolve a cotação diária de "par" (ex.: "USD-BRL") nos últimos
// "dias", em ordem cronológica. A AwesomeAPI devolve do mais novo para o mais
// velho, o primeiro item é a cotação de agora e só ele traz code/codein/name;
// aqui todos os pontos recebem code/codein do par e o name do primeiro.
func (a *AwesomeAPIMoedas) FetchDaily(ctx context.Context, par string, dias int) ([]domain.CurrencyQuote, error) {
	code, codeIn, ok := strings.Cut(par, "-")
	if !ok || code == "" || codeIn == "" {
		return nil, fmt.Errorf("par %q inválido", par)
	}

	var body []moedaAwesome
	u := a.urlDiario + url.PathEscape(par) + "/" + strconv.Itoa(dias)
	if err := httpjson.Get(ctx, a.client, u, a.headers(), &body); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: %s diário vazio", httpjson.ErrCampoAusente, par)
	}

	nome := body[0].Name
	pontos := make([]domain.CurrencyQuote, 0, len(body))
	for i, m := range body {
		q, err := valoresMoeda(fmt.Sprintf("%s[%d]", par, i), m)
		if err != nil {
			return nil, err
		}
		q.Code, q.CodeIn, q.Name = code, codeIn, nome
		pontos = append(pontos, q)
	}
	slices.SortStableFunc(pontos, func(x, y domain.CurrencyQuote) int { return x.QuotedAt.Compare(y.QuotedAt) })
	return pontos, nil
}

func cotacaoMoeda(chave string, m moedaAwesome) (domain.CurrencyQuote, error) {
	if m.Code == "" || m.CodeIn == "" {
		return domain.CurrencyQuote{}, fmt.Errorf("%w: %s.code/codein", httpjson.ErrCampoAusente, chave)
	}
	return valoresMoeda(chave, m)
}

// valoresMoeda lê preços, variações e o instante (code/codein/name só se vierem).
func valoresMoeda(chave string, m moedaAwesome) (domain.CurrencyQuote, error) {
	q := domain.CurrencyQuote{Code: m.Code, CodeIn: m.CodeIn, Name: m.Name}
	var err error
	positivos := []struct {
		campo string
		valor string
		dst   *string
	}{{"bid", m.Bid, &q.Bid}, {"ask", m.Ask, &q.Ask}, {"high", m.High, &q.High}, {"low", m.Low, &q.Low}}
	for _, c := range positivos {
		if *c.dst, err = httpjson.PrecoPositivo(chave+"."+c.campo, c.valor); err != nil {
			return domain.CurrencyQuote{}, err
		}
	}
	if q.VarBid, err = decimalComSinal(chave+".varBid", m.VarBid); err != nil {
		return domain.CurrencyQuote{}, err
	}
	if q.PctChange, err = decimalComSinal(chave+".pctChange", m.PctChange); err != nil {
		return domain.CurrencyQuote{}, err
	}

	seg, err := strconv.ParseInt(m.Timestamp, 10, 64)
	if err != nil || seg <= 0 {
		return domain.CurrencyQuote{}, fmt.Errorf("%w: %s.timestamp = %q", httpjson.ErrJSONInvalido, chave, m.Timestamp)
	}
	q.QuotedAt = time.Unix(seg, 0).UTC()
	return q, nil
}

// decimalComSinal: variações podem ser negativas ("-0.0123").
func decimalComSinal(campo, s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("%w: %s", httpjson.ErrCampoAusente, campo)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", fmt.Errorf("%w: %s = %q", httpjson.ErrPrecoInvalido, campo, s)
	}
	if httpjson.DecimalSimples.MatchString(strings.TrimPrefix(s, "-")) {
		return s, nil
	}
	return r.FloatString(8), nil // notação científica ou sinal "+"
}
