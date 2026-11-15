// Package db embute as migrations no binário, para que a imagem distroless
// (sem shell nem goose CLI) consiga rodar "api migrate".
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
