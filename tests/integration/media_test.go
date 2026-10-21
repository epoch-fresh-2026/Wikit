//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/content"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
)

var pngBody = []byte("\x89PNG\r\n\x1a\nreal-original-bytes")

func (s *suite) prepareUpload(t *testing.T, c identity.Credentials, mime string, size int, key string) media.Upload {
	t.Helper()
	raw, _ := s.req(t, "POST", "uploads", map[string]any{"filename": "original", "size": size, "mime": mime}, c, key, 201)
	return decodeResult[media.Upload](t, raw)
}
func (s *suite) binary(t *testing.T, method, path string, body []byte, c identity.Credentials, headers map[string]string, status int) ([]byte, http.Header) {
	t.Helper()
	r, err := http.NewRequest(method, s.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if c.Bearer != "" {
		r.Header.Set("Authorization", "Bearer "+c.Bearer)
	}
	if c.Session != "" {
		r.AddCookie(&http.Cookie{Name: "wikit_session", Value: c.Session})
	}
	if c.OriginOK {
		r.Header.Set("Origin", "http://localhost:8080")
	}
	r.Header.Set("X-CSRF-Token", c.CSRF)
	r.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != status {
		var failure struct{ Error struct{ Code string } }
		if err := json.Unmarshal(raw, &failure); err != nil {
			t.Fatalf("binary %s: expected %d got %d", path, status, res.StatusCode)
		}
		t.Fatalf("binary %s: expected %d got %d (%s)", path, status, res.StatusCode, failure.Error.Code)
	}
	return raw, res.Header
}
func (s *suite) uploadPNG(t *testing.T, c identity.Credentials, key string) media.Blob {
	t.Helper()
	u := s.prepareUpload(t, c, "image/png", len(pngBody), key)
	raw, _ := s.binary(t, "PUT", u.Path, pngBody, c, nil, 200)
	return decodeResult[media.Blob](t, raw)
}
func TestMEDIA01UploadPublicationAndFixedDownloads(t *testing.T) {
	s := newSuite(t)
	owner, token := s.register(t, "upload_owner")
	otherAccount, otherToken := s.register(t, "upload_other")
	c := identity.Credentials{Bearer: token}
	other := identity.Credentials{Bearer: otherToken}
	admin := identity.Credentials{Bearer: s.adminToken}
	u := s.prepareUpload(t, c, "image/png", len(pngBody), "prepare-png-upload")
	replay := s.prepareUpload(t, c, "image/png", len(pngBody), "prepare-png-upload")
	if u.ID != replay.ID {
		t.Fatal("duplicate reservation")
	}
	s.req(t, "GET", "uploads/"+u.ID, nil, other, "", 404)
	s.binary(t, "PUT", u.Path, pngBody, other, nil, 404)
	raw, _ := s.binary(t, "PUT", u.Path, pngBody, c, nil, 200)
	blob := decodeResult[media.Blob](t, raw)
	raw, _ = s.binary(t, "PUT", u.Path, pngBody, c, nil, 200)
	if decodeResult[media.Blob](t, raw).ID != blob.ID {
		t.Fatal("PUT retry duplicated original")
	}
	changed := append([]byte{}, pngBody...)
	changed[len(changed)-1] = '!'
	s.binary(t, "PUT", u.Path, changed, c, nil, 409)
	s.req(t, "GET", "uploads/"+u.ID, nil, c, "", 200)
	snapshot := content.Snapshot{Type: "media", Title: "Original", Tags: []string{}, Description: ptr("Media"), BlobID: &blob.ID}
	page := s.createPageWithProposal(t, c, "restricted", snapshot, "uploaded-media-page")
	candidateURL := "/api/v1/proposals/" + page.Proposal.ID + "/revisions/" + page.Proposal.CurrentRevision.ID + "/content"
	raw, h := s.binary(t, "GET", candidateURL, nil, c, nil, 200)
	if !bytes.Equal(raw, pngBody) || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, no-store" {
		t.Fatal("incorrect original response")
	}
	s.binary(t, "GET", candidateURL, nil, other, nil, 404)
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "grant-media-reader"), page.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{
		{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read", "propose", "discuss"}},
		{SubjectType: "user", SubjectID: otherAccount.ID, Capabilities: []string{"read"}},
	}}, 1); err != nil {
		t.Fatal(err)
	}
	s.binary(t, "GET", candidateURL, nil, other, nil, 200)
	rev := s.publish(t, admin, page.Proposal, nil, "uploaded-media-publication")
	url := *rev.ContentURL
	s.binary(t, "GET", url, nil, other, map[string]string{"Range": "bytes=0-7"}, 206)
	raw, h = s.binary(t, "GET", url, nil, c, map[string]string{"Range": "bytes=1-4"}, 206)
	if !bytes.Equal(raw, pngBody[1:5]) || !strings.HasPrefix(h.Get("Content-Range"), "bytes 1-4/") {
		t.Fatal("incorrect Range")
	}
	_, h = s.binary(t, "HEAD", url, nil, c, nil, 200)
	if h.Get("Content-Length") == "" {
		t.Fatal("HEAD missing metadata")
	}
	_, h = s.binary(t, "GET", url, nil, c, map[string]string{"Range": "bytes=999999-"}, 416)
	if h.Get("Cache-Control") != "private, no-store" {
		t.Fatal("416 cache policy stripped")
	}
	s.binary(t, "GET", url, nil, c, map[string]string{"If-None-Match": `"` + blob.Digest + `"`}, 304)
	// A new publication never changes the old URL or releases its original.
	newBlob := s.uploadPNG(t, c, "replacement-png-upload")
	snapshot.BlobID = &newBlob.ID
	next := s.createProposal(t, c, page.Page.ID, &rev.ID, snapshot, "replacement-media-proposal")
	s.publish(t, admin, next, &rev.ID, "replacement-media-publish")
	s.binary(t, "GET", "/api/v1/proposals/"+page.Proposal.ID+"/revisions/"+next.CurrentRevision.ID+"/content", nil, c, nil, 404)
	doc := s.createPageWithProposal(t, c, "public", document("Other page", "body"), "download-other-document")
	s.binary(t, "GET", "/api/v1/pages/"+doc.Page.ID+"/revisions/"+rev.ID+"/content", nil, c, nil, 404)
	s.binary(t, "GET", "/api/v1/proposals/"+doc.Proposal.ID+"/revisions/"+doc.Proposal.CurrentRevision.ID+"/content", nil, c, nil, 422)
	raw, _ = s.binary(t, "GET", url, nil, c, nil, 200)
	if !bytes.Equal(raw, pngBody) {
		t.Fatal("old content followed head")
	}
	// Current ACL applies to every old publication/candidate, including HEAD/304.
	if _, err := s.access.UpdatePolicy(context.Background(), requestKey(admin, "revoke-media-download"), page.Page.ID, access.Policy{Visibility: "restricted", Grants: []access.Grant{{SubjectType: "user", SubjectID: owner.ID, Capabilities: []string{"read"}}}}, 2); err != nil {
		t.Fatal(err)
	}
	s.binary(t, "GET", url, nil, other, nil, 404)
	s.binary(t, "GET", candidateURL, nil, other, map[string]string{"If-None-Match": `"` + blob.Digest + `"`}, 404)
	if _, err := s.access.UpdateAccount(context.Background(), requestKey(admin, "disable-media-reader"), otherAccount.ID, "member", "disabled", 1, false); err != nil {
		t.Fatal(err)
	}
	s.binary(t, "GET", url, nil, other, nil, 401)
	execSQL(t, s.db.conn, "UPDATE wikit.page_policies SET status='hidden' WHERE page_id='"+page.Page.ID+"'")
	s.binary(t, "HEAD", url, nil, c, nil, 404)
	s.binary(t, "GET", candidateURL, nil, c, map[string]string{"If-None-Match": `"` + blob.Digest + `"`}, 404)
	s.binary(t, "GET", "/api/v1/blobs/"+blob.ID, nil, c, nil, 404)
}
func TestMEDIA02LimitsMimeFailuresAndOriginalMissing(t *testing.T) {
	s := newSuite(t)
	_, token := s.register(t, "upload_failure_owner")
	c := identity.Credentials{Bearer: token}
	admin := identity.Credentials{Bearer: s.adminToken}
	s.req(t, "POST", "uploads", map[string]any{"filename": "large", "size": 104857601, "mime": "image/png"}, c, "oversize-reservation", 413)
	s.req(t, "POST", "uploads", map[string]any{"filename": "bad", "size": 8, "mime": "image/svg+xml"}, c, "svg-reservation-key", 415)
	u := s.prepareUpload(t, c, "image/png", len(pngBody), "failure-png-upload")
	if _, err := media.New(s.db.store, s.files, s.secrets, 3).Put(context.Background(), c, u.ID, "application/octet-stream", "reduced-limit-request", "test-upload-ip", bytes.NewReader(pngBody)); err != fault.UploadTooLarge {
		t.Fatal("reduced configuration limit ignored", err)
	}
	s.binary(t, "PUT", u.Path, append(pngBody, 'x'), c, nil, 413)
	s.binary(t, "PUT", u.Path, []byte("short"), c, nil, 422)
	bad := bytes.Repeat([]byte{'x'}, len(pngBody))
	s.binary(t, "PUT", u.Path, bad, c, nil, 415)
	s.binary(t, "PUT", u.Path, pngBody, c, map[string]string{"Content-Type": "text/html"}, 415)
	raw, _ := s.binary(t, "PUT", u.Path, pngBody, c, nil, 200)
	blob := decodeResult[media.Blob](t, raw)
	if err := os.Remove(filepath.Join(s.mediaRoot, "objects", blob.ID)); err != nil {
		t.Fatal(err)
	}
	s.req(t, "POST", "pages", map[string]any{"visibility": "public", "snapshot": content.Snapshot{Type: "media", Title: "Missing", Tags: []string{}, Description: ptr(""), BlobID: &blob.ID}}, admin, "missing-original-key", 503)
	var count int
	if err := s.db.conn.QueryRow(context.Background(), "SELECT count(*) FROM wikit.blobs").Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid streams created blobs", count, err)
	}
	second := s.uploadPNG(t, c, "missing-published-upload")
	page := s.createPageWithProposal(t, c, "public", content.Snapshot{Type: "media", Title: "Published then missing", Tags: []string{}, Description: ptr(""), BlobID: &second.ID}, "missing-published-page")
	rev := s.publish(t, admin, page.Proposal, nil, "missing-published-merge")
	if err := os.Remove(filepath.Join(s.mediaRoot, "objects", second.ID)); err != nil {
		t.Fatal(err)
	}
	s.binary(t, "GET", *rev.ContentURL, nil, c, nil, 503)
	s.req(t, "POST", "pages/"+page.Page.ID+"/restore", map[string]any{"revision_id": rev.ID, "expected_head_revision_id": rev.ID}, c, "missing-history-restore", 503)
}

func TestSupportedOriginalFormatsAndPDFDownload(t *testing.T) {
	s := newSuite(t)
	c := identity.Credentials{Bearer: s.adminToken}
	for i, tc := range []struct {
		mime string
		body []byte
	}{
		{"image/jpeg", []byte{255, 216, 255, 224}},
		{"image/png", pngBody},
		{"image/gif", []byte("GIF89a")},
		{"image/webp", []byte("RIFF0000WEBP")},
		{"audio/wav", []byte("RIFF0000WAVE")},
		{"audio/mpeg", []byte{255, 251, 144, 0}},
		{"audio/ogg", []byte("OggS\x00")},
		{"video/mp4", []byte("\x00\x00\x00\x18ftypisom0000")},
		{"video/webm", []byte("\x1a\x45\xdf\xa3\x42\x82\x84webm")},
		{"application/pdf", []byte("%PDF-1.7\n")},
	} {
		key := fmt.Sprintf("format-%d", i)
		u := s.prepareUpload(t, c, tc.mime, len(tc.body), key+"-upload")
		raw, _ := s.binary(t, "PUT", u.Path, tc.body, c, nil, 200)
		b := decodeResult[media.Blob](t, raw)
		p := s.createPageWithProposal(t, c, "public", content.Snapshot{Type: "media", Title: "Format", Tags: []string{}, Description: ptr(""), BlobID: &b.ID}, key+"-page")
		url := "/api/v1/proposals/" + p.Proposal.ID + "/revisions/" + p.Proposal.CurrentRevision.ID + "/content"
		raw, h := s.binary(t, "GET", url, nil, identity.Credentials{}, nil, 200)
		if !bytes.Equal(raw, tc.body) || h.Get("Content-Type") != tc.mime || h.Get("Content-Security-Policy") == "" {
			t.Fatal("original format changed")
		}
		want := "inline"
		if tc.mime == "application/pdf" {
			want = "attachment"
		}
		if !strings.HasPrefix(h.Get("Content-Disposition"), want) {
			t.Fatal("unsafe disposition")
		}
	}
}
