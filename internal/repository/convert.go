package repository

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Os tipos pgtype têm um campo Valid (false = NULL). Convertemos para ponteiros
// (nil = NULL), que é como o domain representa valores opcionais.

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

// stringToNumeric converte o texto decimal exato ("67012.34") para numeric.
func stringToNumeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("decimal %q inválido: %w", s, err)
	}
	return n, nil
}

// numericToString devolve o numeric como texto decimal exato (ex.: "0.85").
func numericToString(n pgtype.Numeric) (string, error) {
	if !n.Valid {
		return "", fmt.Errorf("numeric nulo")
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return "", fmt.Errorf("numeric não finito")
	}
	v, err := n.Value() // driver.Value com a representação textual
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("numeric em formato inesperado: %T", v)
	}
	return s, nil
}
