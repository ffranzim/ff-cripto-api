// Package database cria o pool do Postgres e roda as migrations embutidas.
//
// Cada serviço tem o próprio schema no banco "fundos": o pool usa search_path =
// schema (as queries não qualificam tabelas) e o goose guarda as versões em
// <schema>.goose_db_version, com um lock de migration próprio.
package database

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	gooseDB "github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/franzim/ff-cripto-api/db"
)

var schemaValido = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// NewPool abre o pool com search_path = schema e cria o schema se preciso.
func NewPool(ctx context.Context, url, schema string) (*pgxpool.Pool, error) {
	if !schemaValido.MatchString(schema) {
		return nil, fmt.Errorf("schema %q inválido", schema)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("criar pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schema}.Sanitize()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("criar schema %s: %w", schema, err)
	}
	return pool, nil
}

// lockID: um lock de migration por schema (o padrão do goose seria compartilhado).
func lockID(schema string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("goose:" + schema))
	return int64(h.Sum64() & 0x7fff_ffff_ffff_ffff)
}

// MigrateUp aplica as migrations pendentes de db/migrations (embutidas no binário).
func MigrateUp(ctx context.Context, pool *pgxpool.Pool, schema string, log *slog.Logger) error {
	if !schemaValido.MatchString(schema) {
		return fmt.Errorf("schema %q inválido", schema)
	}
	sqlDB := stdlib.OpenDBFromPool(pool) // goose usa database/sql
	defer sqlDB.Close()

	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	store, err := gooseDB.NewStore(gooseDB.DialectPostgres, schema+".goose_db_version")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(lockID(schema)))
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider("", sqlDB, migrations,
		goose.WithStore(store), goose.WithSessionLocker(locker), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return fmt.Errorf("goose: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	for _, r := range results {
		log.Info("migration aplicada", "schema", schema, "versao", r.Source.Version, "arquivo", r.Source.Path, "duracao_ms", r.Duration.Milliseconds())
	}
	if len(results) == 0 {
		log.Info("nenhuma migration pendente", "schema", schema)
	}
	return nil
}
