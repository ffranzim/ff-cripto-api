// Package interno tem os serviços da própria API (cache, fallback, histórico,
// singleflight). Eles chamam os clientes de service/externo por interfaces.
package interno

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/franzim/ff-cripto-api/internal/domain"
)

// TTLPadraoCotacaoBitcoin vale quando BTC_QUOTE_TTL_SECONDS não é informada.
const TTLPadraoCotacaoBitcoin = 300 * time.Second

// Tempo máximo da busca compartilhada (3 provedores x 5s + gravação no banco).
const timeoutBuscaCotacao = 20 * time.Second

// BitcoinQuoteProvider é satisfeito pelo *FallbackBitcoinQuoteProvider.
type BitcoinQuoteProvider interface {
	Fetch(ctx context.Context) (domain.BitcoinQuote, error)
}

// BitcoinQuoteRepository: GetLast sem nenhuma cotação salva -> domain.ErrNaoEncontrado.
type BitcoinQuoteRepository interface {
	GetLast(ctx context.Context) (domain.BitcoinQuote, error)
	Save(ctx context.Context, q domain.BitcoinQuote) (domain.BitcoinQuote, error)
}

type BitcoinQuoteService struct {
	provider BitcoinQuoteProvider
	repo     BitcoinQuoteRepository
	ttl      time.Duration
	agora    func() time.Time // relógio injetável: trocado nos testes

	// grupo junta buscas simultâneas: enquanto uma está em andamento, as outras
	// requisições esperam o mesmo resultado em vez de chamar as APIs de novo.
	grupo singleflight.Group
}

// NewBitcoinQuoteService: ttl <= 0 usa TTLPadraoCotacaoBitcoin.
func NewBitcoinQuoteService(provider BitcoinQuoteProvider, repo BitcoinQuoteRepository, ttl time.Duration) *BitcoinQuoteService {
	if ttl <= 0 {
		ttl = TTLPadraoCotacaoBitcoin
	}
	return &BitcoinQuoteService{provider: provider, repo: repo, ttl: ttl, agora: time.Now}
}

// GetQuote devolve a última cotação salva enquanto ela tiver menos que o TTL;
// senão (ou com forceRefresh) busca nos provedores e salva um registro novo
// (o histórico fica no banco). Se todos os provedores falharem, devolve a última
// salva com Stale = true; sem nenhuma salva, devolve o erro (ErrIndisponivel).
func (s *BitcoinQuoteService) GetQuote(ctx context.Context, forceRefresh bool) (domain.BitcoinQuoteResult, error) {
	if !forceRefresh {
		ultima, err := s.ultima(ctx)
		if err != nil {
			return domain.BitcoinQuoteResult{}, err
		}
		if ultima != nil && s.agora().Sub(ultima.FetchedAt) < s.ttl {
			return domain.BitcoinQuoteResult{Quote: *ultima}, nil
		}
	}

	// A busca roda desacoplada do ctx de quem chegou primeiro: se aquela
	// request for cancelada, as outras que esperam o mesmo resultado não falham.
	ch := s.grupo.DoChan("btc", func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeoutBuscaCotacao)
		defer cancel()
		return s.atualizar(ctx)
	})

	select {
	case r := <-ch:
		if r.Err != nil {
			return domain.BitcoinQuoteResult{}, r.Err
		}
		return r.Val.(domain.BitcoinQuoteResult), nil
	case <-ctx.Done():
		return domain.BitcoinQuoteResult{}, ctx.Err()
	}
}

func (s *BitcoinQuoteService) atualizar(ctx context.Context) (domain.BitcoinQuoteResult, error) {
	q, errProv := s.provider.Fetch(ctx)
	if errProv == nil {
		salva, err := s.repo.Save(ctx, q)
		if err != nil {
			return domain.BitcoinQuoteResult{}, err
		}
		return domain.BitcoinQuoteResult{Quote: salva}, nil
	}

	// Todos os provedores falharam: serve a última salva, mesmo vencida.
	ultima, err := s.ultima(ctx)
	if err != nil {
		return domain.BitcoinQuoteResult{}, errors.Join(errProv, err)
	}
	if ultima == nil {
		return domain.BitcoinQuoteResult{}, fmt.Errorf("cotação do bitcoin: %w", errProv)
	}
	return domain.BitcoinQuoteResult{Quote: *ultima, Stale: true}, nil
}

// ultima devolve nil (sem erro) quando ainda não há cotação salva.
func (s *BitcoinQuoteService) ultima(ctx context.Context) (*domain.BitcoinQuote, error) {
	q, err := s.repo.GetLast(ctx)
	if errors.Is(err, domain.ErrNaoEncontrado) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}
