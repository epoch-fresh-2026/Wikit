package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
)

type API struct {
	Identity   *identity.Service
	Pages      *pages.Service
	Access     *access.Service
	Secrets    identity.Secrets
	PublicURL  string
	SessionTTL time.Duration
}

func (a API) Handler(health http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health/", health)
	mux.HandleFunc("/api/v1/", a.handle)
	return mux
}
func decode(r *http.Request, dst any, fields ...string) ([]byte, error) {
	return decodeLimit(r, dst, 65536, fields...)
}
func decodeLimit(r *http.Request, dst any, maximum int64, fields ...string) ([]byte, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, fault.BadRequest
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maximum+1))
	if err != nil || int64(len(body)) > maximum || !utf8.Valid(body) {
		return nil, fault.BadRequest
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return nil, fault.BadRequest
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, fault.BadRequest
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil || keys == nil || len(keys) != len(fields) {
		return nil, fault.BadRequest
	}
	for _, field := range fields {
		value, ok := keys[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fault.BadRequest
		}
	}
	return body, nil
}
func response(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	if value == nil {
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// A failed response write cannot be recovered after headers were sent.
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
func failure(w http.ResponseWriter, err error, requestID string) {
	code := "internal_error"
	status := 500
	var business fault.Error
	if errors.As(err, &business) {
		code = string(business)
		switch business {
		case fault.BadRequest:
			status = 400
		case fault.Unauthenticated:
			status = 401
		case fault.Forbidden:
			status = 403
		case fault.NotFound:
			status = 404
		case fault.Stale, fault.State, fault.Idempotency:
			status = 409
		case fault.Invalid:
			status = 422
		case fault.Limited:
			status = 429
			w.Header().Set("Retry-After", "60")
		}
	}
	response(w, status, map[string]any{"error": map[string]string{"code": code, "message": code, "request_id": requestID}})
}
func (a API) cookie(w http.ResponseWriter, value string, clear bool) {
	u, _ := url.Parse(a.PublicURL)
	c := &http.Cookie{Name: "wikit_session", Value: value, Path: "/", HttpOnly: true, Secure: u.Scheme == "https", SameSite: http.SameSiteLaxMode, MaxAge: int(a.SessionTTL.Seconds())}
	if clear {
		c.MaxAge = -1
		c.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, c)
}
func (a API) credentials(r *http.Request) (identity.Credentials, error) {
	c := identity.Credentials{Write: r.Method != "GET" && r.Method != "HEAD", OriginOK: r.Header.Get("Origin") == strings.TrimSuffix(a.PublicURL, "/"), CSRF: r.Header.Get("X-CSRF-Token")}
	if cookies := r.CookiesNamed("wikit_session"); len(cookies) > 1 {
		return c, fault.BadRequest
	} else if len(cookies) == 1 {
		c.Session = cookies[0].Value
	}
	header := r.Header.Values("Authorization")
	if len(header) > 1 {
		return c, fault.BadRequest
	}
	if len(header) == 1 {
		fields := strings.Fields(header[0])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || len(fields[1]) > 256 {
			return c, fault.Unauthenticated
		}
		c.Bearer = fields[1]
	}
	if len(c.Session) > 256 || len(c.CSRF) > 256 {
		return c, fault.BadRequest
	}
	return c, nil
}
func (a API) handle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	id, err := a.Secrets.ID()
	if err != nil {
		failure(w, err, "")
		return
	}
	w.Header().Set("X-Request-ID", id)
	c, err := a.credentials(r)
	if err != nil {
		failure(w, err, id)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	var result any
	status := 200
	var body []byte
	switch {
	case r.Method == "POST" && (path == "auth/register" || path == "auth/login"):
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_, err = decode(r, &input, "username", "password")
		if err != nil {
			break
		}
		if path == "auth/register" {
			var account identity.Account
			var token string
			account, token, err = a.Identity.Register(r.Context(), input.Username, input.Password, ip, id, c.OriginOK)
			result = struct {
				Account identity.Account `json:"account"`
				Token   string           `json:"token"`
			}{account, token}
			status = 201
		} else {
			var login identity.Login
			login, err = a.Identity.Login(r.Context(), input.Username, input.Password, ip, id, c.OriginOK)
			if err == nil {
				a.cookie(w, login.Session, false)
			}
			result = struct {
				Account identity.Account `json:"account"`
				CSRF    string           `json:"csrf_token"`
			}{login.Account, login.CSRF}
		}
	case r.Method == "GET" && path == "auth/me":
		result, err = a.Identity.Current(r.Context(), c)
	case r.Method == "POST" && path == "auth/logout":
		c.SessionOnly = true
		err = a.Identity.Logout(r.Context(), c, id)
		if err == nil {
			a.cookie(w, "", true)
		}
		status = 204
	case r.Method == "GET" && path == "auth/token":
		var token identity.Token
		token, err = a.Identity.TokenStatus(r.Context(), c)
		result = struct {
			Active     bool      `json:"active"`
			Generation int64     `json:"generation"`
			CreatedAt  time.Time `json:"created_at"`
		}{token.Digest != "", token.Generation, token.CreatedAt}
	case r.Method == "POST" && (path == "auth/token" || path == "auth/token/revoke" || path == "auth/password"):
		var input struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		fields := []string{"current_password"}
		if path == "auth/password" {
			fields = append(fields, "new_password")
		}
		_, err = decode(r, &input, fields...)
		if err != nil {
			break
		}
		if path != "auth/password" && input.NewPassword != "" {
			err = fault.BadRequest
			break
		}
		op := "rotate"
		if path == "auth/password" {
			op = "password"
		} else if path == "auth/token/revoke" {
			op = "revoke"
		}
		var plain string
		var token identity.Token
		plain, token, err = a.Identity.ChangeSecret(r.Context(), c, input.CurrentPassword, input.NewPassword, op, id)
		if op == "rotate" {
			result = struct {
				Token      string    `json:"token"`
				Generation int64     `json:"generation"`
				CreatedAt  time.Time `json:"created_at"`
			}{plain, token.Generation, token.CreatedAt}
		} else {
			status = 204
			if err == nil && op == "password" {
				a.cookie(w, "", true)
			}
		}
	case r.Method == "GET" && (path == "admin/accounts" || path == "admin/groups" || path == "admin/audit"):
		limit := 20
		if value := r.URL.Query().Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil {
				err = fault.BadRequest
				break
			}
		}
		result, err = a.Access.List(r.Context(), c, strings.TrimPrefix(path, "admin/"), r.URL.Query().Get("cursor"), limit)
	case r.Method == "PATCH" && strings.HasPrefix(path, "admin/accounts/"):
		var input struct {
			Role    string `json:"role"`
			Status  string `json:"status"`
			Version *int64 `json:"expected_auth_version"`
		}
		body, err = decode(r, &input, "role", "status", "expected_auth_version")
		if err != nil {
			break
		}
		if input.Version == nil {
			err = fault.BadRequest
			break
		}
		result, err = a.Access.UpdateAccount(r.Context(), request(r, c, id, body), strings.TrimPrefix(path, "admin/accounts/"), input.Role, input.Status, *input.Version)
	case (r.Method == "POST" && path == "admin/groups") || (r.Method == "PUT" && strings.HasPrefix(path, "admin/groups/")):
		var input struct {
			Name    string   `json:"name"`
			Members []string `json:"member_ids"`
			Version *int64   `json:"expected_version"`
		}
		fields := []string{"name"}
		if r.Method == "PUT" {
			fields = append(fields, "member_ids", "expected_version")
		}
		body, err = decode(r, &input, fields...)
		if err != nil {
			break
		}
		groupID := ""
		version := int64(0)
		if r.Method == "PUT" {
			groupID = strings.TrimPrefix(path, "admin/groups/")
			if input.Version == nil || input.Members == nil {
				err = fault.BadRequest
				break
			}
			version = *input.Version
		} else {
			if input.Version != nil || input.Members != nil {
				err = fault.BadRequest
				break
			}
			status = 201
		}
		result, err = a.Access.SaveGroup(r.Context(), request(r, c, id, body), groupID, input.Name, input.Members, version)
	case (r.Method == "GET" || r.Method == "PUT") && strings.HasPrefix(path, "pages/") && strings.HasSuffix(path, "/policy"):
		pageID := strings.TrimSuffix(strings.TrimPrefix(path, "pages/"), "/policy")
		if !identity.ValidID(pageID) {
			err = fault.NotFound
			break
		}
		if r.Method == "GET" {
			result, err = a.Access.GetPolicy(r.Context(), c, pageID)
		} else {
			var input struct {
				Visibility string         `json:"visibility"`
				Grants     []access.Grant `json:"grants"`
				Version    *int64         `json:"expected_policy_version"`
			}
			body, err = decode(r, &input, "visibility", "grants", "expected_policy_version")
			if err != nil {
				break
			}
			if input.Version == nil || input.Grants == nil {
				err = fault.BadRequest
				break
			}
			result, err = a.Access.UpdatePolicy(r.Context(), request(r, c, id, body), pageID, access.Policy{Visibility: input.Visibility, Grants: input.Grants}, *input.Version)
		}
	default:
		if a.Pages != nil {
			result, status, err = a.pageRoute(r, c, id, path, ip)
		} else {
			err = fault.NotFound
		}
	}
	if err != nil {
		failure(w, err, id)
		return
	}
	// Governance returns serialized public results to preserve idempotent responses.
	if raw, ok := result.([]byte); ok {
		result = json.RawMessage(raw)
	}
	response(w, status, result)
}
func request(r *http.Request, c identity.Credentials, id string, body []byte) access.Request {
	return access.Request{Credentials: c, ID: id, Key: r.Header.Get("Idempotency-Key"), Hash: identity.Digest(string(body))}
}
