// Package health coordinates readiness independently of transport and storage.
package health

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("service unavailable")

// Check is owned by the consumer. Implementations must honor cancellation and
// must not change durable application data. Errors are never sent to clients.
type Check interface{ Check(context.Context) error }

type Service struct{ database, media Check }

func New(database, media Check) *Service { return &Service{database: database, media: media} }

func (s *Service) Ready(ctx context.Context) error {
	for _, dependency := range []Check{s.database, s.media} {
		if ctx.Err() != nil {
			return ErrUnavailable
		}
		if err := dependency.Check(ctx); err != nil {
			return ErrUnavailable
		}
	}
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
