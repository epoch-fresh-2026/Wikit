package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMeasurementsAreOptInAndRequestLocal(t *testing.T) {
	tracer := measurementTracer{}
	plain := context.Background()
	if tracer.TraceQueryStart(plain, nil, pgx.TraceQueryStartData{}) != plain {
		t.Fatal("unmeasured request allocated tracing context")
	}
	first, a := Measure(plain)
	second, b := Measure(plain)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := tracer.TraceAcquireStart(first, nil, pgxpool.TraceAcquireStartData{})
			tracer.TraceAcquireEnd(ctx, nil, pgxpool.TraceAcquireEndData{})
			ctx = tracer.TraceQueryStart(first, nil, pgx.TraceQueryStartData{SQL: "secret query", Args: []any{"secret value"}})
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
		}()
	}
	wg.Wait()
	ctx := tracer.TraceQueryStart(second, nil, pgx.TraceQueryStartData{})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	if a.Queries.Load() != 40 || b.Queries.Load() != 1 || b.PoolNanos.Load() != 0 {
		t.Fatal("cross-request or lost measurements")
	}
}
