// Package config lê a configuração das variáveis de ambiente.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SchemaPadrao: as tabelas do serviço ficam neste schema do banco "fundos".
const SchemaPadrao = "ff_cripto"

var schemaValido = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Provedores de cotação do Bitcoin aceitos em BTC_QUOTE_PROVIDERS e a ordem padrão.
var (
	provedoresBitcoinValidos = map[string]bool{"coingecko": true, "binance": true, "awesomeapi": true}
	provedoresBitcoinPadrao  = []string{"coingecko", "binance", "awesomeapi"}
)

type Config struct {
	DatabaseURL string
	DBSchema    string // DB_SCHEMA (padrão ff_cripto): search_path do pool e das migrations
	Host        string // 127.0.0.1 por padrão: com hostNetwork, só o Caddy deve alcançar a API
	Port        int

	// Cotação do Bitcoin.
	CoinGeckoAPIKey      string        // COINGECKO_API_KEY; vazio = API pública (limite menor)
	AwesomeAPIKey        string        // AWESOMEAPI_API_KEY; vazio = API pública (cota por IP)
	BitcoinQuoteTTL      time.Duration // BTC_QUOTE_TTL_SECONDS (padrão 300)
	BitcoinQuoteProvider []string      // BTC_QUOTE_PROVIDERS, ordem do fallback
}

func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Host:        getenv("HOST", "127.0.0.1"),
		DBSchema:    getenv("DB_SCHEMA", SchemaPadrao),

		CoinGeckoAPIKey: os.Getenv("COINGECKO_API_KEY"),
		AwesomeAPIKey:   os.Getenv("AWESOMEAPI_API_KEY"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL é obrigatória")
	}
	if !schemaValido.MatchString(cfg.DBSchema) {
		return Config{}, fmt.Errorf("DB_SCHEMA %q inválido: use ^[a-z_][a-z0-9_]*$", cfg.DBSchema)
	}

	port, err := strconv.Atoi(getenv("PORT", "8082"))
	if err != nil || port <= 0 || port > 65535 {
		return Config{}, errors.New("PORT inválida")
	}
	cfg.Port = port

	ttl, err := strconv.Atoi(getenv("BTC_QUOTE_TTL_SECONDS", "300"))
	if err != nil || ttl <= 0 {
		return Config{}, errors.New("BTC_QUOTE_TTL_SECONDS deve ser um inteiro > 0")
	}
	cfg.BitcoinQuoteTTL = time.Duration(ttl) * time.Second

	cfg.BitcoinQuoteProvider, err = parseProvedores(os.Getenv("BTC_QUOTE_PROVIDERS"))
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// parseProvedores lê "coingecko,binance,awesomeapi" (a ordem é a do fallback).
// Vazio = ordem padrão. Nome desconhecido ou repetido é erro.
func parseProvedores(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return append([]string(nil), provedoresBitcoinPadrao...), nil
	}
	vistos := map[string]bool{}
	var out []string
	for _, nome := range strings.Split(v, ",") {
		nome = strings.ToLower(strings.TrimSpace(nome))
		if !provedoresBitcoinValidos[nome] {
			return nil, fmt.Errorf("BTC_QUOTE_PROVIDERS: provedor %q desconhecido (use coingecko, binance, awesomeapi)", nome)
		}
		if vistos[nome] {
			return nil, fmt.Errorf("BTC_QUOTE_PROVIDERS: provedor %q repetido", nome)
		}
		vistos[nome] = true
		out = append(out, nome)
	}
	return out, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
