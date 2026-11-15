package httpapi

import (
	"context"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }
