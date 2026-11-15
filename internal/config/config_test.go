package config

import (
	"slices"
	"testing"
	"time"
)

func TestLoad_CotacaoBitcoin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")

	t.Run("padrões", func(t *testing.T) {
		t.Setenv("BTC_QUOTE_TTL_SECONDS", "")
		t.Setenv("BTC_QUOTE_PROVIDERS", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BitcoinQuoteTTL != 300*time.Second {
			t.Errorf("TTL = %v, queria 5m", cfg.BitcoinQuoteTTL)
		}
		if want := []string{"coingecko", "binance", "awesomeapi"}; !slices.Equal(cfg.BitcoinQuoteProvider, want) {
			t.Errorf("provedores = %v, queria %v", cfg.BitcoinQuoteProvider, want)
		}
	})

	t.Run("valores informados", func(t *testing.T) {
		t.Setenv("BTC_QUOTE_TTL_SECONDS", "60")
		t.Setenv("BTC_QUOTE_PROVIDERS", " AwesomeAPI , coingecko")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BitcoinQuoteTTL != time.Minute {
			t.Errorf("TTL = %v", cfg.BitcoinQuoteTTL)
		}
		if want := []string{"awesomeapi", "coingecko"}; !slices.Equal(cfg.BitcoinQuoteProvider, want) {
			t.Errorf("provedores = %v, queria %v", cfg.BitcoinQuoteProvider, want)
		}
	})

	invalidos := map[string][2]string{
		"TTL zero":             {"0", ""},
		"TTL não numérico":     {"5m", ""},
		"provedor inexistente": {"", "coingecko,kraken"},
		"provedor repetido":    {"", "binance,binance"},
		"item vazio":           {"", "coingecko,,binance"},
	}
	for name, v := range invalidos {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BTC_QUOTE_TTL_SECONDS", v[0])
			t.Setenv("BTC_QUOTE_PROVIDERS", v[1])
			if _, err := Load(); err == nil {
				t.Error("queria erro")
			}
		})
	}
}
