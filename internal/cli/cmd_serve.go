// serve --mcp: the reference tool set over MCP (change add-mcp-server, #32).

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/mcp"
	"github.com/nodelogicau/episodes-cli/internal/prov"
	"github.com/nodelogicau/episodes-cli/internal/store"
)

func (a *app) serveCmd() *cobra.Command {
	var useMCP, conversation bool
	var author, harness, mdl string
	cmd := &cobra.Command{
		Use:   "serve --mcp [--author A] [--harness H] [--model M] [--conversation-tools]",
		Short: "Serve this workspace to an MCP client over stdio: the format's reference tool set",
		Long: `Run a Model Context Protocol server over stdio, bound to one workspace
(found as every verb finds it). It exposes the Episodes Format's reference
tool set, one tool per verb: episode_record_inline, episode_record_ref,
act_attest, episode_observe, episode_redact, episode_show, episode_list,
episode_query, episode_thread, validate and workspace_status. A tool runs its
verb, so its result is the verb's --json output and its refusals are the
verb's.

The author, harness and clock are bound here, and no tool takes them: the
author from --author, then $` + prov.EnvAuthor + `, then defaults.source.author; the
harness from --harness, then $` + prov.EnvHarness + `, then defaults.source.harness,
then the client's name. A writing tool may pass model. $` + EnvNow + ` is ignored:
every timestamp and id comes from the clock.

conversation_start and conversation_turn are listed only with
--conversation-tools. A harness should record turns from its own transcript,
through a hook calling the conversation verbs, not by a model re-typing them.

Resources episode://{id} and episode://{id}/thread read as show and thread,
and subscribers are told when observe or redact changes one. Stdout carries
only the protocol; diagnostics go to stderr.`,
		Args: cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			if !useMCP {
				return usageErr("serve requires --mcp (the only transport)")
			}
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			if a.getenv(EnvNow) != "" {
				fmt.Fprintf(a.stderr, "episodes serve: %s is ignored; timestamps and ids come from the clock\n", EnvNow)
			}
			srv := a.newMCPServer(ws, author, harness, mdl, conversation)
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			fmt.Fprintf(a.stderr, "episodes %s serving %s over stdio (found by %s)\n", version, ws.Root, ws.FoundBy)
			return srv.Run(ctx, &sdk.StdioTransport{})
		}),
	}
	cmd.Flags().BoolVar(&useMCP, "mcp", false, "speak MCP over stdio (required)")
	cmd.Flags().StringVar(&author, "author", "", "source.author for every write (default: $"+prov.EnvAuthor+", then defaults.source.author)")
	cmd.Flags().StringVar(&harness, "harness", "", "source.harness for every write (default: $"+prov.EnvHarness+", then defaults.source.harness, then the client's name)")
	cmd.Flags().StringVar(&mdl, "model", "", "source.model when a call gives none (default: $"+prov.EnvModel+", then defaults.source.model)")
	cmd.Flags().BoolVar(&conversation, "conversation-tools", false, "also list conversation_start and conversation_turn")
	return cmd
}

// newMCPServer binds a server to ws: attribution resolved once (design D4),
// and each call run in-process with EPISODES_NOW hidden (D3).
func (a *app) newMCPServer(ws *store.Workspace, author, harness, mdl string, conversation bool) *mcp.Server {
	src := prov.Resolve(ws.Config.Defaults.Source.Source(), prov.Explicit{Author: author, Harness: harness, Model: mdl})
	getenv := a.getenv
	hide := func(k string) string {
		if k == EnvNow {
			return ""
		}
		return getenv(k)
	}
	return mcp.New(mcp.Options{
		Root: ws.Root, Version: version, Author: src.Author, Harness: src.Harness, Model: src.Model,
		ConversationTools: conversation,
		Run: func(args []string, stdout, stderr io.Writer) int {
			return ExecuteWith(Options{Getenv: hide}, args, strings.NewReader(""), stdout, stderr, func() bool { return true })
		},
	})
}
