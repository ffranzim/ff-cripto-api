package interno

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// TimeoutPadrao é o tempo máximo de cada provedor (não da cadeia inteira).
const TimeoutPadrao = 5 * time.Second

// NamedBitcoinQuoteProvider é satisfeito pelos clientes externos
// (*coingecko.CoinGecko, *binance.Binance, *awesomeapi.AwesomeAPI)
// e pelo próprio *FallbackBitcoinQuoteProvider.
type NamedBitcoinQuoteProvider interface {
	Name() string
	Fetch(ctx context.Context) (domain.BitcoinQuote, error)
}

// ErrTodosFalharam embrulha domain.ErrIndisponivel (vira 503 na API).
var ErrTodosFalharam = fmt.Errorf("%w: todos os provedores de cotação falharam", domain.ErrIndisponivel)

// FallbackBitcoinQuoteProvider tenta os provedores na ordem recebida e devolve
// o primeiro sucesso. A ordem vem de quem monta (main, via BTC_QUOTE_PROVIDERS).
type FallbackBitcoinQuoteProvider struct {
	providers []NamedBitcoinQuoteProvider
	timeout   time.Duration
	log       *slog.Logger
}

// NewFallbackBitcoinQuoteProvider: timeout <= 0 usa TimeoutPadrao.
func NewFallbackBitcoinQuoteProvider(log *slog.Logger, timeout time.Duration, providers ...NamedBitcoinQuoteProvider) *FallbackBitcoinQuoteProvider {
	if timeout <= 0 {
		timeout = TimeoutPadrao
	}
	return &FallbackBitcoinQuoteProvider{providers: providers, timeout: timeout, log: log}
}

func (*FallbackBitcoinQuoteProvider) Name() string { return "fallback" }

// Fetch: falha = timeout, status não-2xx, JSON inválido, campo ausente ou preço <= 0
// (os provedores devolvem erro em todos esses casos). Cada falha é logada; se
// todas falharem, o erro agrega os motivos e satisfaz errors.Is(err, ErrTodosFalharam).
func (f *FallbackBitcoinQuoteProvider) Fetch(ctx context.Context) (domain.BitcoinQuote, error) {
	falhas := make([]error, 0, len(f.providers))

	for _, p := range f.providers {
		// Contexto cancelado por quem chamou: não adianta tentar o próximo.
		if err := ctx.Err(); err != nil {
			return domain.BitcoinQuote{}, err
		}

		q, err := f.tentar(ctx, p)
		if err == nil {
			return q, nil
		}

		f.log.WarnContext(ctx, "provedor de cotação do bitcoin falhou", "provedor", p.Name(), "err", err)
		falhas = append(falhas, fmt.Errorf("%s: %w", p.Name(), err))
	}

	// %w duplo: errors.Is funciona tanto com ErrTodosFalharam quanto com cada motivo.
	return domain.BitcoinQuote{}, fmt.Errorf("%w: %w", ErrTodosFalharam, errors.Join(falhas...))
}

func (f *FallbackBitcoinQuoteProvider) tentar(ctx context.Context, p NamedBitcoinQuoteProvider) (domain.BitcoinQuote, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	q, err := p.Fetch(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.BitcoinQuote{}, fmt.Errorf("timeout de %s: %w", f.timeout, err)
		}
		return domain.BitcoinQuote{}, err
	}
	return q, nil
}
