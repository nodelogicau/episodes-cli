package mcp

import (
	"context"
	"encoding/json"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const scheme = "episode://"

// addResources registers episode://{id} and episode://{id}/thread (design
// D8). The scheme is the server's own and never written into an episode.
func (s *Server) addResources() {
	s.srv.AddResourceTemplate(&sdk.ResourceTemplate{Name: "episode", URITemplate: scheme + "{id}", MIMEType: "application/json",
		Description: "One episode, as episode_show returns it."}, s.readResource)
	s.srv.AddResourceTemplate(&sdk.ResourceTemplate{Name: "thread", URITemplate: scheme + "{id}/thread", MIMEType: "application/json",
		Description: "A container's children in order, as episode_thread returns it."}, s.readResource)
}

func (s *Server) readResource(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
	uri := req.Params.URI
	id, ok := strings.CutPrefix(uri, scheme)
	verb := "show"
	if rest, thread := strings.CutSuffix(id, "/thread"); thread {
		id, verb = rest, "thread"
	}
	if !ok || id == "" || strings.Contains(id, "/") {
		return nil, sdk.ResourceNotFoundError(uri)
	}
	out, errb, code := s.exec([]string{verb, "--workspace=" + s.o.Root, "--json", "--", id})
	if code != 0 {
		res := result(out, errb, code)
		if sc, _ := res.StructuredContent.(map[string]any); sc != nil {
			if e, _ := sc["error"].(map[string]any); e != nil && e["code"] == "not_found" {
				return nil, sdk.ResourceNotFoundError(uri)
			}
		}
		return nil, &resourceErr{text(res)}
	}
	return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: "application/json", Text: strings.TrimSpace(string(out))}}}, nil
}

type resourceErr struct{ msg string }

func (e *resourceErr) Error() string { return e.msg }

func text(r *sdk.CallToolResult) string {
	for _, c := range r.Content {
		if t, ok := c.(*sdk.TextContent); ok {
			return t.Text
		}
	}
	return "error"
}

// notify tells subscribers that an episode changed after observe or redact,
// and its container's thread when it has one.
func (s *Server) notify(ctx context.Context, id string) {
	uris := []string{scheme + id}
	out, _, code := s.exec([]string{"show", "--workspace=" + s.o.Root, "--json", "--", id})
	if code == 0 {
		var m map[string]any
		if json.Unmarshal(out, &m) == nil {
			if ep, _ := m["episode"].(map[string]any); ep != nil {
				if c, _ := ep["container"].(string); c != "" {
					uris = append(uris, scheme+c+"/thread")
				}
			}
		}
	}
	for _, u := range uris {
		_ = s.srv.ResourceUpdated(ctx, &sdk.ResourceUpdatedNotificationParams{URI: u})
	}
}
