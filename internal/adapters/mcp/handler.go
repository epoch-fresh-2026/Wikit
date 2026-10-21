// Package mcp maps the generated tool catalog directly to Core use cases.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contract "github.com/epoch-fresh-2026/Wikit/api"
	"github.com/epoch-fresh-2026/Wikit/internal/core/access"
	"github.com/epoch-fresh-2026/Wikit/internal/core/discussions"
	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
	"github.com/epoch-fresh-2026/Wikit/internal/core/identity"
	"github.com/epoch-fresh-2026/Wikit/internal/core/media"
	"github.com/epoch-fresh-2026/Wikit/internal/core/pages"
	"github.com/epoch-fresh-2026/Wikit/internal/core/proposals"
	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Services struct {
	Identity    *identity.Service
	Access      *access.Service
	Pages       *pages.Service
	Proposals   *proposals.Service
	Discussions *discussions.Service
	Media       *media.Service
	Search      *search.Service
	Secrets     identity.Secrets
	PublicURL   string
}
type Handler struct {
	server *sdk.Server
	http   http.Handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.http.ServeHTTP(noStoreWriter{w}, r)
}

// Keep the application's sensitive-response policy when the SDK sets its SSE
// cache headers; Unwrap retains ResponseController's native capabilities.
type noStoreWriter struct{ http.ResponseWriter }

func (w noStoreWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(code)
}
func (w noStoreWriter) Write(body []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(body)
}
func (w noStoreWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w noStoreWriter) FlushError() error {
	w.Header().Set("Cache-Control", "no-store")
	return http.NewResponseController(w.ResponseWriter).Flush()
}
func (w noStoreWriter) Flush() {
	if err := w.FlushError(); err != nil {
		return
	}
}
func (h *Handler) Close() error {
	var result error
	for session := range h.server.Sessions() {
		result = errors.Join(result, session.Close())
	}
	return result
}

type catalogTool struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Input       json.RawMessage     `json:"inputSchema"`
	Output      json.RawMessage     `json:"outputSchema"`
	Annotations sdk.ToolAnnotations `json:"annotations"`
	Operation   string              `json:"x-rest-operation"`
}

func New(s Services) (*Handler, error) {
	origin, err := url.Parse(s.PublicURL)
	if err != nil || origin.Host == "" {
		return nil, errors.New("invalid MCP public origin")
	}
	if s.Identity == nil || s.Access == nil || s.Pages == nil || s.Proposals == nil || s.Discussions == nil || s.Media == nil || s.Search == nil || s.Secrets == nil {
		return nil, errors.New("missing MCP Core service")
	}
	var catalog struct {
		Tools []catalogTool `json:"tools"`
	}
	if err := json.Unmarshal(contract.MCPCatalog(), &catalog); err != nil {
		return nil, errors.New("invalid MCP catalog")
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "Wikit", Version: "0.1.0"}, nil)
	seen := map[string]bool{}
	for _, tool := range catalog.Tools {
		if !knownOperation(tool.Operation) || tool.Name != "wikit_"+tool.Operation || seen[tool.Name] {
			return nil, errors.New("MCP catalog dispatch mismatch")
		}
		seen[tool.Name] = true
		if _, err := compile(tool.Input); err != nil {
			return nil, err
		}
		shape, err := shapeSchema(tool.Input)
		if err != nil {
			return nil, err
		}
		output, err := compile(tool.Output)
		if err != nil {
			return nil, err
		}
		var inMap, outMap map[string]any
		if err := json.Unmarshal(tool.Input, &inMap); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tool.Output, &outMap); err != nil {
			return nil, err
		}
		if inMap["type"] != "object" {
			return nil, errors.New("MCP input schema must be an object")
		}
		server.AddTool(&sdk.Tool{Name: tool.Name, Description: tool.Description, InputSchema: inMap, OutputSchema: outMap, Annotations: &tool.Annotations}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			requestID := ""
			if req.Extra != nil {
				requestID = req.Extra.Header.Get("X-Wikit-Request-ID")
			}
			reject := func(err error) (*sdk.CallToolResult, error) {
				raw, marshalErr := json.Marshal(fault.Response(err, requestID))
				if marshalErr != nil {
					return nil, errors.New("error serialization failed")
				}
				return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}}, nil
			}
			if req.Extra == nil || req.Extra.TokenInfo == nil {
				return reject(fault.Unauthenticated)
			}
			var value any
			if !utf8.Valid(req.Params.Arguments) || json.Unmarshal(req.Params.Arguments, &value) != nil {
				return reject(fault.BadRequest)
			}
			if err := shape.Validate(value); err != nil {
				return reject(fault.BadRequest)
			}
			var args arguments
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return reject(fault.BadRequest)
			}
			header := strings.Fields(req.Extra.Header.Get("Authorization"))
			if len(header) != 2 {
				return reject(fault.Unauthenticated)
			}
			c := identity.Credentials{Bearer: header[1]}
			raw := args.Body
			maxBytes := 65536
			switch tool.Operation {
			case "page_create", "proposal_create", "proposal_update", "proposal_merge", "proposal_rebase":
				maxBytes = 5 << 20
			case "issue_create", "comment_create":
				maxBytes = 1 << 20
			}
			if len(raw) > maxBytes {
				return reject(fault.BadRequest)
			}
			r := access.Request{Credentials: c, ID: requestID, Key: args.Key, Hash: access.JSONHash(raw)}
			out, err := s.invoke(ctx, tool.Operation, args, r, req.Extra.Header.Get("X-Wikit-Client-IP"))
			if err != nil {
				return reject(err)
			}
			if bytes, ok := out.([]byte); ok {
				out = json.RawMessage(bytes)
			}
			response := map[string]any{"data": out}
			if out == nil {
				response = map[string]any{"ok": true}
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				return reject(errors.New("output serialization failed"))
			}
			var normalized any
			// The schema validator expects JSON numbers decoded as float64. Keep
			// the original bytes on the wire so large integers are not rounded.
			if err := json.Unmarshal(encoded, &normalized); err != nil {
				return reject(err)
			}
			if err := output.Validate(normalized); err != nil {
				return reject(errors.New("output contract mismatch"))
			}
			return &sdk.CallToolResult{StructuredContent: json.RawMessage(encoded), Content: []sdk.Content{&sdk.TextContent{Text: "ok"}}}, nil
		})
	}
	if len(seen) != len(operations) {
		return nil, errors.New("incomplete MCP catalog")
	}
	transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{JSONResponse: true, SessionTimeout: 15 * time.Minute, MaxRequestBodyBytes: 6 << 20, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
	verified := auth.RequireBearerToken(func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		p, err := s.Identity.Authenticate(ctx, identity.Credentials{Bearer: token})
		if err != nil {
			var business fault.Error
			if errors.As(err, &business) {
				return nil, auth.ErrInvalidToken
			}
			return nil, errors.New("authentication unavailable")
		}
		return &auth.TokenInfo{UserID: p.ID + ":" + strconv.FormatInt(p.AuthVersion, 10) + ":" + strconv.FormatInt(p.Generation, 10)}, nil
	}, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(transport)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		w.Header().Set("Cache-Control", "no-store")
		if !allowedHost(r.Host, origin) || len(r.Header.Values("Origin")) > 1 || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != strings.TrimSuffix(s.PublicURL, "/") || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "forbidden", 403)
			return
		}
		if len(r.Header.Values("Authorization")) > 1 {
			http.Error(w, "invalid_request", 400)
			return
		}
		id, err := s.Secrets.ID()
		if err != nil {
			http.Error(w, "internal_error", 500)
			return
		}
		w.Header().Set("X-Request-ID", id)
		r = r.Clone(r.Context())
		r.Header.Set("X-Wikit-Request-ID", id)
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		r.Header.Set("X-Wikit-Client-IP", ip)
		verified.ServeHTTP(w, r)
	})
	return &Handler{server: server, http: h}, nil
}
func allowedHost(host string, origin *url.URL) bool {
	if strings.EqualFold(host, origin.Host) {
		return true
	}
	local := func(value string) bool {
		if strings.EqualFold(value, "localhost") {
			return true
		}
		ip := net.ParseIP(value)
		return ip != nil && ip.IsLoopback()
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	return local(origin.Hostname()) && local(hostname)
}
func compile(raw []byte) (*jsonschema.Resolved, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("invalid MCP schema")
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, errors.New("MCP schema compilation failed")
	}
	return resolved, nil
}
func shapeSchema(raw []byte) (*jsonschema.Resolved, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var prune func(any)
	prune = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, key := range []string{"enum", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "minItems", "maxItems", "uniqueItems", "pattern", "format", "default"} {
				delete(x, key)
			}
			for _, child := range x {
				prune(child)
			}
		case []any:
			for _, child := range x {
				prune(child)
			}
		}
	}
	prune(value)
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return compile(body)
}
