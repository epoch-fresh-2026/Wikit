package postgres

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Measurements is opt-in, request-local telemetry. It never retains SQL or values.
type Measurements struct {
	Queries       atomic.Int64
	SQLNanos      atomic.Int64
	PoolNanos     atomic.Int64
	LockWaitNanos atomic.Int64
	LockHeldNanos atomic.Int64
}
type measureKey struct{}
type queryTimeKey struct{}
type acquireTimeKey struct{}

func Measure(ctx context.Context) (context.Context, *Measurements) {
	m := &Measurements{}
	return context.WithValue(ctx, measureKey{}, m), m
}
func measurements(ctx context.Context) *Measurements {
	m, _ := ctx.Value(measureKey{}).(*Measurements)
	return m
}

type measurementTracer struct{}

func (measurementTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	if m := measurements(ctx); m != nil {
		m.Queries.Add(1)
		return context.WithValue(ctx, queryTimeKey{}, time.Now())
	}
	return ctx
}
func (measurementTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if m := measurements(ctx); m != nil {
		if start, ok := ctx.Value(queryTimeKey{}).(time.Time); ok {
			m.SQLNanos.Add(time.Since(start).Nanoseconds())
		}
	}
}
func (measurementTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	if measurements(ctx) != nil {
		return context.WithValue(ctx, acquireTimeKey{}, time.Now())
	}
	return ctx
}
func (measurementTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if m := measurements(ctx); m != nil {
		if start, ok := ctx.Value(acquireTimeKey{}).(time.Time); ok {
			m.PoolNanos.Add(time.Since(start).Nanoseconds())
		}
	}
}
