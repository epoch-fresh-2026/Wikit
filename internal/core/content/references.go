package content

import (
	"context"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
)

type ReferenceReader interface {
	access.Authorization
	Page(context.Context, string) (Page, error)
	Revision(context.Context, string, string) (Published, error)
	Blob(context.Context, string) (Blob, error)
}

// CheckReferences checks current target authorization and the exact published
// media/blob inside the caller's transaction. A URL never grants access.
func CheckReferences(ctx context.Context, tx ReferenceReader, c identity.Credentials, refs []MediaRef) error {
	for _, ref := range refs {
		if _, _, err := access.Authorize(ctx, tx, c, ref.PageID, "read", false); err != nil {
			return err
		}
		p, err := tx.Page(ctx, ref.PageID)
		if err != nil {
			return err
		}
		if p.Type != "media" || p.HeadRevisionID == nil {
			return fault.Invalid
		}
		v, err := tx.Revision(ctx, ref.PageID, ref.RevisionID)
		if err != nil {
			return err
		}
		if v.Snapshot.BlobID == nil {
			return fault.Invalid
		}
		b, err := tx.Blob(ctx, *v.Snapshot.BlobID)
		if err != nil {
			return err
		}
		if b.State != "ready" {
			return fault.State
		}
	}
	return nil
}
