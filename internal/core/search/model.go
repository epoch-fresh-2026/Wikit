// Package search owns authorized search and recoverable projection use cases.
package search

import (
	"context"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type Source struct {
	Page     content.Page
	Policy   access.Policy
	Snapshot content.Snapshot
}
type Document struct {
	ID            string   `json:"page_id"`
	Revision      string   `json:"revision_id"`
	PolicyVersion int64    `json:"policy_version"`
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	Body          string   `json:"body"`
	Visibility    string   `json:"visibility"`
	Users         []string `json:"read_users"`
	Groups        []string `json:"read_groups"`
}
type Candidate struct {
	ID            string `json:"page_id"`
	Revision      string `json:"revision_id"`
	PolicyVersion int64  `json:"policy_version"`
}
type Candidates struct {
	Hits []Candidate
	More bool
}
type Filter struct {
	Actor  string
	Groups []string
	Admin  bool
	Tag    string
}
type Index interface {
	Ensure(context.Context, string) error
	Put(context.Context, string, []Document) (int64, error)
	Delete(context.Context, string, []string) (int64, error)
	Wait(context.Context, int64) error
	Search(context.Context, string, string, Filter, int, int) (Candidates, error)
}
type Text interface {
	NFC(string) string
	Fold(string) string
	Plain(string) (string, error)
}
type Tx interface {
	access.Authorization
	ActiveIndex(context.Context) (string, error)
	SearchSources(context.Context, []string) ([]Source, error)
	Repair(context.Context, string) error
	Cursor(context.Context, string, string, string, int) (int, error)
	NewCursor(context.Context, string, string, string, int, int) (string, error)
}
type Repository interface {
	WithinSearch(context.Context, func(Tx) error) error
}
type Event struct {
	Sequence int64
	Page     string
	Task     *int64
	Target   string
	Attempts int
}
type Export struct {
	IDs       []string
	Watermark int64
}
type Stats struct {
	Pending       int64 `json:"pending"`
	OldestSeconds int64 `json:"oldest_seconds"`
	Failures      int64 `json:"failures"`
}
type Queue interface {
	ActiveIndex(context.Context) (string, error)
	SearchSource(context.Context, string) (Source, error)
	NextEvent(context.Context) (*Event, error)
	SaveTask(context.Context, int64, string, int64) error
	FailEvent(context.Context, int64, string, bool, time.Duration) error
	CompleteEvent(context.Context, int64) error
	Export(context.Context, int64, bool) (Export, error)
	SwitchIndex(context.Context, string) error
	RetryEvents(context.Context) error
}
type WorkerRepository interface {
	WithSearchWorker(context.Context, func(Queue) error) error
	SearchStats(context.Context) (Stats, error)
}
type Service struct {
	repo    Repository
	index   Index
	text    Text
	secrets identity.Secrets
}

func New(repo Repository, index Index, text Text, secrets identity.Secrets) *Service {
	return &Service{repo, index, text, secrets}
}

type Worker struct {
	repo    WorkerRepository
	index   Index
	text    Text
	secrets identity.Secrets
}

func NewWorker(repo WorkerRepository, index Index, text Text, secrets identity.Secrets) *Worker {
	return &Worker{repo, index, text, secrets}
}
