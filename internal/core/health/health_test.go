package health

import (
	"context"
	"errors"
	"testing"
)

type checkFunc func(context.Context) error

func (f checkFunc) Check(ctx context.Context) error { return f(ctx) }

func TestReadiness(t *testing.T) {
	for _, fail := range []int{-1, 0, 1} {
		calls := 0
		check := checkFunc(func(context.Context) error {
			current := calls
			calls++
			if current == fail {
				return errors.New("secret")
			}
			return nil
		})
		err := New(check, check).Ready(context.Background())
		if fail == -1 && err != nil {
			t.Fatal(err)
		}
		if fail >= 0 && !errors.Is(err, ErrUnavailable) {
			t.Fatalf("unexpected error: %v", err)
		}
		if fail == 0 && calls != 1 {
			t.Fatal("continued after failure")
		}
	}
}

func TestCanceledReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	check := checkFunc(func(context.Context) error { t.Fatal("called after cancellation"); return nil })
	if !errors.Is(New(check, check).Ready(ctx), ErrUnavailable) {
		t.Fatal("cancellation ignored")
	}
}
