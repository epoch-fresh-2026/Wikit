package search

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

// ErrTaskFailed distinguishes a terminal remote task from an unknown network outcome.
var ErrTaskFailed = errors.New("search task failed")

func (w *Worker) Process(ctx context.Context, limit int) error {
	if limit < 1 || limit > 1000 {
		return fault.Invalid
	}
	return w.repo.WithSearchWorker(ctx, func(q Queue) error {
		index, err := q.ActiveIndex(ctx)
		if err != nil {
			return err
		}
		if err := w.index.Ensure(ctx, index); err != nil {
			event, lookupErr := q.NextEvent(ctx)
			if lookupErr != nil {
				return lookupErr
			}
			if event != nil {
				return w.failed(ctx, q, *event, "submit_unavailable", false)
			}
			return fault.SearchUnavailable
		}
		for n := 0; n < limit; n++ {
			event, err := q.NextEvent(ctx)
			if err != nil {
				return err
			}
			if event == nil {
				return nil
			}
			task := event.Task
			if task == nil {
				source, err := q.SearchSource(ctx, event.Page)
				if err != nil && !errors.Is(err, fault.NotFound) {
					return err
				}
				var doc *Document
				if err == nil {
					doc, err = project(source, w.text)
					if err != nil {
						return err
					}
				}
				var uid int64
				if doc == nil {
					uid, err = w.index.Delete(ctx, index, []string{event.Page})
				} else {
					uid, err = w.index.Put(ctx, index, []Document{*doc})
				}
				if err != nil {
					return w.failed(ctx, q, *event, "submit_unavailable", false)
				}
				if err := q.SaveTask(ctx, event.Sequence, index, uid); err != nil {
					return err
				}
				task = &uid
			}
			if err := w.index.Wait(ctx, *task); err != nil {
				return w.failed(ctx, q, *event, "task_unavailable", errors.Is(err, ErrTaskFailed))
			}
			if err := q.CompleteEvent(ctx, event.Sequence); err != nil {
				return err
			}
		}
		return nil
	})
}
func (w *Worker) failed(ctx context.Context, q Queue, e Event, code string, clear bool) error {
	// Persist retry state even when the caller's operation deadline elapsed.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	delay := time.Second * time.Duration(1<<min(e.Attempts, 8))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	if err := q.FailEvent(cleanup, e.Sequence, code, clear, delay); err != nil {
		return err
	}
	return fault.SearchUnavailable
}
func (w *Worker) Rebuild(ctx context.Context) (string, error) {
	var target string
	err := w.repo.WithSearchWorker(ctx, func(q Queue) error {
		id, err := w.secrets.ID()
		if err != nil {
			return err
		}
		target = "wikit_pages_" + id
		if err := w.index.Ensure(ctx, target); err != nil {
			return fault.SearchUnavailable
		}
		export, err := q.Export(ctx, 0, true)
		if err != nil {
			return err
		}
		if err := w.export(ctx, q, target, export.IDs); err != nil {
			return err
		}
		changes, err := q.Export(ctx, export.Watermark, false)
		if err != nil {
			return err
		}
		if err := w.export(ctx, q, target, changes.IDs); err != nil {
			return err
		}
		return q.SwitchIndex(ctx, target)
	})
	return target, err
}
func (w *Worker) export(ctx context.Context, q Queue, target string, ids []string) error {
	docs := []Document{}
	deleted := []string{}
	batchBytes := 0
	flush := func() error {
		if len(docs) > 0 {
			task, err := w.index.Put(ctx, target, docs)
			if err != nil {
				return fault.SearchUnavailable
			}
			if err := w.index.Wait(ctx, task); err != nil {
				return fault.SearchUnavailable
			}
			docs = nil
			batchBytes = 0
		}
		if len(deleted) > 0 {
			task, err := w.index.Delete(ctx, target, deleted)
			if err != nil {
				return fault.SearchUnavailable
			}
			if err := w.index.Wait(ctx, task); err != nil {
				return fault.SearchUnavailable
			}
			deleted = nil
		}
		return nil
	}
	for _, id := range ids {
		source, err := q.SearchSource(ctx, id)
		if err != nil && !errors.Is(err, fault.NotFound) {
			return err
		}
		var doc *Document
		if err == nil {
			doc, err = project(source, w.text)
			if err != nil {
				return err
			}
		}
		if doc == nil {
			deleted = append(deleted, id)
		} else {
			raw, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			// Keep each ordinary batch within 4 MiB including JSON separators.
			// An individual valid large document still travels in its own request.
			if len(docs) > 0 && batchBytes+len(raw)+1 > 4<<20 {
				if err := flush(); err != nil {
					return err
				}
			}
			docs = append(docs, *doc)
			batchBytes += len(raw) + 1
		}
		if len(docs)+len(deleted) >= 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}
func (w *Worker) Retry(ctx context.Context) error {
	return w.repo.WithSearchWorker(ctx, func(q Queue) error { return q.RetryEvents(ctx) })
}
func (w *Worker) Status(ctx context.Context) (Stats, error) {
	return w.repo.SearchStats(ctx)
}
