// Package httpjson tem o que os clientes das APIs externas (pastas irmãs, uma por
// fornecedor) compartilham: GET de JSON, os erros de cada tentativa e a
// validação dos preços que chegam como texto.
package httpjson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
)

// Erros de cada tentativa; o fallback loga e agrega todos eles.
var (
	ErrHTTP          = errors.New("status HTTP de erro")
	ErrJSONInvalido  = errors.New("JSON inválido")
	ErrCampoAusente  = errors.New("campo ausente")
	ErrPrecoInvalido = errors.New("preço inválido")
)

// Respostas dessas APIs têm poucos bytes; o limite barra uma resposta gigante.
const tamanhoMaximoResposta = 1 << 20 // 1 MiB

// Get faz o GET e decodifica o corpo em dst. Status fora de 2xx -> ErrHTTP;
// corpo que não é JSON (ou não casa com dst) -> ErrJSONInvalido.
func Get(ctx context.Context, client *http.Client, url string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err // inclui o timeout (context.DeadlineExceeded)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: %d", ErrHTTP, resp.StatusCode)
	}

	dec := json.NewDecoder(io.LimitReader(resp.Body, tamanhoMaximoResposta))
	dec.UseNumber() // números chegam como json.Number (texto), sem passar por float64
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrJSONInvalido, err)
	}
	return nil
}

// DecimalSimples casa decimais sem sinal e sem notação científica ("123.45").
var DecimalSimples = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// PrecoPositivo valida o preço vindo da API e o devolve como texto decimal.
// Vazio (campo ausente ou null) -> ErrCampoAusente; não numérico ou <= 0 -> ErrPrecoInvalido.
func PrecoPositivo(campo, s string) (string, error) {
	return decimalValido(campo, s, false)
}

// ValorNaoNegativo é o PrecoPositivo que aceita zero (market cap, volume).
func ValorNaoNegativo(campo, s string) (string, error) {
	return decimalValido(campo, s, true)
}

func decimalValido(campo, s string, aceitaZero bool) (string, error) {
	if s == "" {
		return "", fmt.Errorf("%w: %s", ErrCampoAusente, campo)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", fmt.Errorf("%w: %s = %q", ErrPrecoInvalido, campo, s)
	}
	if r.Sign() < 0 || (r.Sign() == 0 && !aceitaZero) {
		minimo := "> 0"
		if aceitaZero {
			minimo = ">= 0"
		}
		return "", fmt.Errorf("%w: %s = %s (deve ser %s)", ErrPrecoInvalido, campo, s, minimo)
	}
	if DecimalSimples.MatchString(s) {
		return s, nil
	}
	// Notação científica ("6.7e4") ou sinal "+": normaliza para decimal simples.
	return r.FloatString(8), nil
}
