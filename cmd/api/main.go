// Ponto de entrada: só composição (injeção de dependências manual) e ciclo de vida.
//
//	api           → sobe o servidor HTTP
//	api migrate   → aplica as migrations pendentes e sai
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/franzim/ff-cripto-api/internal/config"
	"github.com/franzim/ff-cripto-api/internal/database"
	"github.com/franzim/ff-cripto-api/internal/domain"
	httpapi "github.com/franzim/ff-cripto-api/internal/http"
	"github.com/franzim/ff-cripto-api/internal/repository"
	"github.com/franzim/ff-cripto-api/internal/service/externo/awesomeapi"
	"github.com/franzim/ff-cripto-api/internal/service/externo/binance"
	"github.com/franzim/ff-cripto-api/internal/service/externo/coingecko"
	"github.com/franzim/ff-cripto-api/internal/service/interno"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// main só trata o erro final; toda a lógica fica em run(), que devolve error.
	if err := run(log); err != nil {
		log.Error("encerrando com erro", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// ctx é cancelado quando o processo recebe SIGINT/SIGTERM (podman stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL, cfg.DBSchema)
	if err != nil {
		return err
	}
	defer pool.Close()

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return database.MigrateUp(ctx, pool, cfg.DBSchema, log)
	}

	// Composição: repository + service/externo -> service/interno -> http
	bitcoin, bitcoinFontes, err := novosServicosBitcoin(cfg, log, repository.NewBitcoinQuoteRepository(pool))
	if err != nil {
		return err
	}
	// Histórico (gráfico) vem só da CoinGecko, com a mesma chave da cotação.
	bitcoinHistorico := interno.NewBitcoinHistoryService(
		coingecko.NewCoinGeckoHistorico(cfg.CoinGeckoAPIKey, &http.Client{Timeout: 15 * time.Second}),
		repository.NewBitcoinHistoryRepository(pool),
		interno.TTLHistoricoBitcoin,
	)

	awesomeMoedas := awesomeapi.NewAwesomeAPIMoedas(cfg.AwesomeAPIKey, &http.Client{Timeout: 10 * time.Second})
	moedas := interno.NewCurrencyQuoteService(awesomeMoedas, domain.FonteAwesomeAPI, cfg.BitcoinQuoteTTL, interno.TimeoutPadrao)
	moedasHistorico := interno.NewCurrencyHistoryService(awesomeMoedas, awesomeapi.ParesMoedas, domain.FonteAwesomeAPI,
		interno.TTLHistoricoMoedas, interno.TimeoutPadrao)

	router := httpapi.NewRouter(httpapi.Deps{
		Logger:           log,
		DB:               pool,
		Bitcoin:          bitcoin,
		BitcoinFontes:    bitcoinFontes,
		BitcoinHistorico: bitcoinHistorico,
		Moedas:           moedas,
		MoedasHistorico:  moedasHistorico,
	})

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe bloqueia, então roda numa goroutine (≈ uma tarefa concorrente leve).
	// O erro volta pelo channel errCh (≈ um Stream/Completer do Dart).
	errCh := make(chan error, 1)
	go func() {
		log.Info("api ouvindo", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	// select espera o que acontecer primeiro: falha do servidor ou sinal de parada.
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		log.Info("sinal recebido, encerrando")
	}

	// Graceful shutdown: para de aceitar conexões e espera as requests em andamento.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// novosServicosBitcoin monta a cadeia de fallback na ordem de BTC_QUOTE_PROVIDERS
// (GET /quotes/bitcoin) e o serviço por provedor (GET /quotes/bitcoin/{fonte}),
// que expõe os três provedores independentemente dessa variável.
func novosServicosBitcoin(cfg config.Config, log *slog.Logger, repo *repository.BitcoinQuoteRepository) (*interno.BitcoinQuoteService, *interno.BitcoinProviderQuoteService, error) {
	// O timeout de 5s por provedor é aplicado pelo fallback (via ctx); o do client
	// é só uma rede de segurança.
	client := &http.Client{Timeout: 10 * time.Second}

	if cfg.CoinGeckoAPIKey == "" {
		log.Warn("COINGECKO_API_KEY vazia: CoinGecko será chamada sem chave (limite menor)")
	}
	if cfg.AwesomeAPIKey == "" {
		log.Warn("AWESOMEAPI_API_KEY vazia: AwesomeAPI será chamada sem chave (cota por IP, costuma dar 429 na VPS)")
	}
	porNome := map[string]interno.NamedBitcoinQuoteProvider{
		domain.FonteCoinGecko:  coingecko.NewCoinGecko(cfg.CoinGeckoAPIKey, client),
		domain.FonteBinance:    binance.NewBinance(client),
		domain.FonteAwesomeAPI: awesomeapi.NewAwesomeAPI(cfg.AwesomeAPIKey, client),
	}

	providers := make([]interno.NamedBitcoinQuoteProvider, 0, len(cfg.BitcoinQuoteProvider))
	for _, nome := range cfg.BitcoinQuoteProvider {
		p, ok := porNome[nome]
		if !ok {
			return nil, nil, fmt.Errorf("provedor de cotação %q desconhecido", nome)
		}
		providers = append(providers, p)
	}

	fallback := interno.NewFallbackBitcoinQuoteProvider(log, interno.TimeoutPadrao, providers...)

	porFonte := make(map[string]interno.BitcoinQuoteProvider, len(porNome))
	for nome, p := range porNome {
		porFonte[nome] = p
	}

	return interno.NewBitcoinQuoteService(fallback, repo, cfg.BitcoinQuoteTTL),
		interno.NewBitcoinProviderQuoteService(porFonte, cfg.BitcoinQuoteTTL, interno.TimeoutPadrao),
		nil
}
