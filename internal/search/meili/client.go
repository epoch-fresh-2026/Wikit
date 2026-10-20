// Package meili translates the internal search port to Meilisearch's HTTP API.
package meili

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
)

type Client struct {
	base, key string
	http      *http.Client
}

func New(base, key string) *Client {
	return &Client{strings.TrimRight(base, "/"), key, &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("search redirect refused") }}}
}
func (c *Client) call(ctx context.Context, method, path string, body, out any) (int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, fault.SearchUnavailable
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(data))
	if err != nil {
		return 0, fault.SearchUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return 0, fault.SearchUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, fault.SearchUnavailable
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
			return res.StatusCode, fault.SearchUnavailable
		}
	}
	return res.StatusCode, nil
}
func path(index string) string { return "/indexes/" + url.PathEscape(index) }

type task struct {
	UID *int64 `json:"taskUid"`
}

func (c *Client) submit(ctx context.Context, method, path string, body any) (int64, error) {
	var out task
	_, err := c.call(ctx, method, path, body, &out)
	if err != nil {
		return 0, err
	}
	if out.UID == nil || *out.UID < 0 {
		return 0, fault.SearchUnavailable
	}
	return *out.UID, nil
}
func (c *Client) Wait(ctx context.Context, uid int64) error {
	for {
		var state struct {
			Status string `json:"status"`
		}
		status, err := c.call(ctx, "GET", "/tasks/"+strconv.FormatInt(uid, 10), nil, &state)
		if err != nil {
			if status == 404 {
				return search.ErrTaskFailed
			}
			return err
		}
		switch state.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			return search.ErrTaskFailed
		case "enqueued", "processing":
		default:
			return fault.SearchUnavailable
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fault.SearchUnavailable
		case <-timer.C:
		}
	}
}
func (c *Client) Ensure(ctx context.Context, index string) error {
	status, err := c.call(ctx, "GET", path(index), nil, nil)
	if err != nil {
		if status != 404 {
			return err
		}
		uid, err := c.submit(ctx, "POST", "/indexes", map[string]any{"uid": index, "primaryKey": "page_id"})
		if err != nil {
			return err
		}
		if err := c.Wait(ctx, uid); err != nil {
			return err
		}
	}
	wanted := map[string]any{
		"searchableAttributes": []string{"title", "tags", "body"},
		"displayedAttributes":  []string{"page_id", "revision_id", "policy_version"},
		"filterableAttributes": []string{"visibility", "read_users", "read_groups", "tags"},
		"rankingRules":         []string{"attribute", "words", "typo", "proximity", "sort", "exactness"},
		"localizedAttributes":  []any{map[string]any{"attributePatterns": []string{"title", "tags", "body"}, "locales": []string{"zho", "eng"}}},
		"pagination":           map[string]any{"maxTotalHits": 1000000},
	}
	var current map[string]any
	if _, err := c.call(ctx, "GET", path(index)+"/settings", nil, &current); err != nil {
		return err
	}
	// Compare JSON-normalized values so an unchanged configuration creates no task.
	raw, err := json.Marshal(wanted)
	if err != nil {
		return err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return err
	}
	same := true
	for key, value := range normalized {
		if !reflect.DeepEqual(current[key], value) {
			same = false
			break
		}
	}
	if same {
		return nil
	}
	uid, err := c.submit(ctx, "PATCH", path(index)+"/settings", wanted)
	if err != nil {
		return err
	}
	return c.Wait(ctx, uid)
}
func (c *Client) Put(ctx context.Context, index string, docs []search.Document) (int64, error) {
	return c.submit(ctx, "POST", path(index)+"/documents", docs)
}
func (c *Client) Delete(ctx context.Context, index string, ids []string) (int64, error) {
	return c.submit(ctx, "POST", path(index)+"/documents/delete-batch", ids)
}
func quote(s string) string { raw, _ := json.Marshal(s); return string(raw) }
func (c *Client) Search(ctx context.Context, index, q string, f search.Filter, offset, limit int) (search.Candidates, error) {
	filter := []string{}
	if !f.Admin {
		acl := []string{"visibility = \"public\""}
		if f.Actor != "" {
			acl = append(acl, "read_users = "+quote(f.Actor))
			for _, group := range f.Groups {
				acl = append(acl, "read_groups = "+quote(group))
			}
		}
		filter = append(filter, "("+strings.Join(acl, " OR ")+")")
	}
	if f.Tag != "" {
		filter = append(filter, "tags = "+quote(f.Tag))
	}
	var out struct {
		Hits []search.Candidate `json:"hits"`
	}
	_, err := c.call(ctx, "POST", path(index)+"/search", map[string]any{"q": q, "filter": strings.Join(filter, " AND "), "offset": offset, "limit": limit, "attributesToRetrieve": []string{"page_id", "revision_id", "policy_version"}, "locales": []string{"zho", "eng"}}, &out)
	if err != nil {
		return search.Candidates{}, err
	}
	return search.Candidates{Hits: out.Hits, More: len(out.Hits) == limit}, nil
}
