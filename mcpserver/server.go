// Package mcpserver serves Isley to AI agents over the Model Context
// Protocol at /mcp (streamable HTTP).
//
// It is a thin layer over the service package, like /api/v1, but shaped
// for agents: a dozen or so task-sized tools instead of one per endpoint,
// names accepted where people use names ("Water", "Flower"), and results
// as a one-line summary followed by compact JSON.
//
// Clients authenticate with the same scoped API keys as /api/v1, as a
// bearer token. A key sees only the tools its scope allows: read keys get
// the read tools, write and admin keys get the write tools as well. There
// are no delete tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isley/logger"
	"isley/scope"
	"isley/service"
)

// Path is where the server is mounted.
const Path = "/mcp"

// IsPath reports whether path belongs to the MCP server.
func IsPath(path string) bool {
	return path == Path || strings.HasPrefix(path, Path+"/")
}

// Key is the API key a request authenticated with.
type Key struct {
	ID     int
	Name   string
	Scopes scope.Set
}

// KeyVerifier checks a presented key. It returns nil, nil when the key is
// not valid.
type KeyVerifier func(ctx context.Context, plaintext string) (*Key, error)

// Options configures the handler.
type Options struct {
	Version string
	// Enabled is asked on every request; when it says no, /mcp answers 404
	// as if it weren't there. It follows the setting without a restart.
	Enabled func() bool
}

const keyInfo = "isley.key"

// Handler returns the /mcp endpoint.
func Handler(svc *service.Services, verify KeyVerifier, opts Options) http.Handler {
	read := newServer(svc, opts.Version, scope.Read)
	write := newServer(svc, opts.Version, scope.Write)

	// Stateless: every request stands alone, so nothing is kept between
	// calls and any instance can answer. Isley's tools need no session.
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		k := keyFrom(auth.TokenInfoFromContext(r.Context()))
		if k != nil && k.Scopes.Allows(scope.Write) {
			return write
		}
		return read
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	authed := auth.RequireBearerToken(tokenVerifier(verify), &auth.RequireBearerTokenOptions{
		Scopes:                 []string{string(scope.Read)},
		AllowMissingExpiration: true,
	})(mcpHandler)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.Enabled != nil && !opts.Enabled() {
			http.NotFound(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
}

// tokenVerifier adapts the API key check to the SDK's bearer middleware.
// The token's scopes are the levels the key allows (a write key has read
// and write), so the middleware can require read of every caller.
func tokenVerifier(verify KeyVerifier) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		k, err := verify(ctx, token)
		if err != nil {
			logger.Log.WithError(err).Error("MCP: API key check failed")
			return nil, errors.New("could not check the API key")
		}
		if k == nil {
			return nil, fmt.Errorf("%w: the API key is not valid", auth.ErrInvalidToken)
		}
		var scopes []string
		for _, s := range []scope.Scope{scope.Read, scope.Write, scope.Admin} {
			if k.Scopes.Allows(s) {
				scopes = append(scopes, string(s))
			}
		}
		return &auth.TokenInfo{
			Scopes: scopes,
			UserID: k.Name,
			Extra:  map[string]any{keyInfo: k},
		}, nil
	}
}

func keyFrom(info *auth.TokenInfo) *Key {
	if info == nil {
		return nil
	}
	k, _ := info.Extra[keyInfo].(*Key)
	return k
}

const instructions = `Isley is a grow journal: plants, the strains they were grown from, the zones (tents, rooms) they live in, and the sensors in those zones.

Start with get_overview for what is growing now and the latest climate in each zone. list_reference_data has the names of statuses, activity types and measurement metrics, which the write tools accept by name.

Times: results give date-times in RFC 3339 with the instance's UTC offset, and the overview says which zone that is. Date-times you send without an offset are read in the instance zone. Dates (start_date) are YYYY-MM-DD.

Plant age: the start day is day 1; week 1 is days 1-7.`

// newServer builds the tools a key with the given level can use.
func newServer(svc *service.Services, version string, level scope.Scope) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "isley", Title: "Isley", Version: version},
		&mcp.ServerOptions{
			Instructions: instructions,
			// What a client may list depends on its key, so lists must
			// not be shared between clients.
			SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.CacheScope = "private" },
		})
	t := &tools{svc: svc}
	t.addReadTools(s)
	if level == scope.Write {
		t.addWriteTools(s)
	}
	return s
}

type tools struct {
	svc *service.Services
}

// result is what a tool returns: a one-line summary for people and
// agents skimming, and the data.
type result struct {
	summary string
	data    any
}

// handler is a tool's work.
type handler[In any] func(ctx context.Context, in In) (result, error)

// addTool registers a tool that needs the given scope. The scope is
// checked again on every call, though a key without it never sees the
// tool listed.
func addTool[In any](s *mcp.Server, t *mcp.Tool, need scope.Scope, h handler[In]) {
	readOnly := need == scope.Read
	if t.Annotations == nil {
		t.Annotations = &mcp.ToolAnnotations{}
	}
	t.Annotations.ReadOnlyHint = readOnly
	if !readOnly {
		f := false
		t.Annotations.DestructiveHint = &f
	}
	f := false
	t.Annotations.OpenWorldHint = &f

	mcp.AddTool(s, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		var k *Key
		if req.Extra != nil {
			k = keyFrom(req.Extra.TokenInfo)
		}
		if k == nil || !k.Scopes.Allows(need) {
			return errorResult("this API key does not have the " + string(need) + " scope"), nil, nil
		}
		started := time.Now()
		res, err := h(ctx, in)
		if !readOnly {
			logWrite(k, t.Name, in, err, time.Since(started))
		}
		if err != nil {
			return toolError(t.Name, err), nil, nil
		}
		return textResult(res)
	})
}

func textResult(r result) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(r.data)
	if err != nil {
		return nil, nil, fmt.Errorf("encode result: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: r.summary + "\n" + string(b)},
	}}, nil, nil
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// toolError reports a service error to the agent in words it can act on;
// anything unexpected is logged and reported without details.
func toolError(tool string, err error) *mcp.CallToolResult {
	if e, ok := service.AsError(err); ok {
		return errorResult(e.Error())
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errorResult("the request was cancelled")
	}
	logger.Log.WithError(err).WithField("tool", tool).Error("MCP tool failed")
	return errorResult("internal error: the tool failed; the server log has details")
}

// logWrite records who changed what, the way API writes are recorded.
func logWrite(k *Key, tool string, in any, err error, took time.Duration) {
	args, _ := json.Marshal(in)
	entry := logger.Log.WithFields(map[string]any{
		"key_id": k.ID, "key_name": k.Name, "tool": tool, "args": string(args), "took": took.String(),
	})
	if err != nil {
		entry.WithError(err).Warn("MCP write failed")
		return
	}
	entry.Info("MCP write")
}
