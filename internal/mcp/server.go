// Package mcp serves a workspace over the Model Context Protocol, exposing
// the Episodes Format's Reference Tool Set (change add-mcp-server, #32).
//
// A tool runs its verb in-process through a Runner (design D1): the verb's
// --json output is the result and its error envelope the refusal, so the
// two cannot drift. This package never imports internal/cli; serve passes
// the Runner in.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Runner runs the CLI with args and returns its exit code, as cli.Execute
// does, with the server's environment applied.
type Runner func(args []string, stdout, stderr io.Writer) int

// Options bind a server when it starts (design D4).
type Options struct {
	Root    string // the workspace root, passed as --workspace to every call
	Version string
	// Author, Harness and Model are the resolved source. An empty harness
	// falls back to the connected client's name.
	Author, Harness, Model string
	// ConversationTools lists conversation_start and conversation_turn
	// (design D9).
	ConversationTools bool
	Run               Runner
}

// Server is one workspace served over MCP.
type Server struct {
	o   Options
	mu  sync.Mutex // one call at a time (design D7)
	srv *sdk.Server
}

// New builds a server with its tools and resources registered.
func New(o Options) *Server {
	s := &Server{o: o}
	s.srv = sdk.NewServer(&sdk.Implementation{Name: "episodes", Version: o.Version}, &sdk.ServerOptions{
		Instructions:       instructions,
		SubscribeHandler:   func(context.Context, *sdk.SubscribeRequest) error { return nil },
		UnsubscribeHandler: func(context.Context, *sdk.UnsubscribeRequest) error { return nil },
	})
	for i := range toolDefs {
		d := &toolDefs[i]
		if d.Conversation && !o.ConversationTools {
			continue
		}
		d.add(s, d)
	}
	s.addResources()
	return s
}

// SDK is the underlying server, for tests that connect in memory.
func (s *Server) SDK() *sdk.Server { return s.srv }

// Run serves until the client disconnects or ctx ends.
func (s *Server) Run(ctx context.Context, t sdk.Transport) error { return s.srv.Run(ctx, t) }

const instructions = `Episodes records what happened, when, from whose standpoint, and where the record of it lives, one YAML file per episode. Episodes are never revised: observe and redact are the only changes to an existing file. Record, never interpret: an episode says what happened; a judgement about it belongs elsewhere. Recall (episode_list, episode_query) before recording a reference to content that may already be recorded. The author, harness and clock are bound by this server: no tool takes them. Every result equals the CLI's --json output for the same verb.`

// exec runs one verb under the lock and returns its streams and exit code.
func (s *Server) exec(args []string) (stdout, stderr []byte, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out, errb bytes.Buffer
	code = s.o.Run(args, &out, &errb)
	return out.Bytes(), errb.Bytes(), code
}

// verbArgs is a verb's full argument list: the verb, its flags, the bound
// workspace and --json, attribution for a writer, then -- and positionals
// (design D2).
func (s *Server) verbArgs(req *sdk.CallToolRequest, d *Tool, b built) []string {
	args := append([]string{}, d.Verb...)
	args = append(args, b.flags...)
	args = append(args, "--workspace="+s.o.Root, "--json")
	if d.Writes {
		if s.o.Author != "" {
			args = append(args, "--author="+s.o.Author)
		}
		if h := s.harness(req); h != "" {
			args = append(args, "--harness="+h)
		}
		if !b.set["model"] && s.o.Model != "" {
			args = append(args, "--model="+s.o.Model)
		}
	}
	if len(b.pos) > 0 {
		args = append(args, "--")
		args = append(args, b.pos...)
	}
	return args
}

// harness is the bound harness, else the connected client's name.
func (s *Server) harness(req *sdk.CallToolRequest) string {
	if s.o.Harness != "" {
		return s.o.Harness
	}
	if req != nil && req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			return strings.TrimSpace(p.ClientInfo.Name)
		}
	}
	return ""
}

// result turns a verb's run into a tool result (design D1).
func result(stdout, stderr []byte, code int) *sdk.CallToolResult {
	var env struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if code != 0 && json.Unmarshal(stderr, &env) == nil && env.Error != nil {
		return &sdk.CallToolResult{
			IsError:           true,
			Content:           []sdk.Content{&sdk.TextContent{Text: env.Error.Code + ": " + env.Error.Message}},
			StructuredContent: map[string]any{"error": map[string]any{"code": env.Error.Code, "message": env.Error.Message}},
		}
	}
	var m map[string]any
	if err := json.Unmarshal(stdout, &m); err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = "no result"
		}
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "runtime: " + msg}}}
	}
	// A verdict with its report on stdout and no error envelope, such as
	// validate's exit 4, is a result.
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: strings.TrimSpace(string(stdout))}},
		StructuredContent: m,
	}
}

// call runs a tool: build its arguments, run its verb, notify subscribers.
func (s *Server) call(ctx context.Context, req *sdk.CallToolRequest, d *Tool, in any) *sdk.CallToolResult {
	b, err := build(in, s.o.Root)
	if err != nil {
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "usage: " + err.Error()}},
			StructuredContent: map[string]any{"error": map[string]any{"code": "usage", "message": err.Error()}}}
	}
	out, errb, code := s.exec(s.verbArgs(req, d, b))
	res := result(out, errb, code)
	if d.Notify && !res.IsError && len(b.pos) == 1 {
		s.notify(ctx, b.pos[0])
	}
	return res
}
