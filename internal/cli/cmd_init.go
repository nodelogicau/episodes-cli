// Origin: nodelogicau/intentions-cli internal/cli/cmd_init.go @ b72a395454aa
// (the init verb's shape; episodes takes --author, --harness and --zone, and
// never takes a zone from the host)

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nodelogicau/episodes-cli/internal/prov"
	"github.com/nodelogicau/episodes-cli/internal/store"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

func (a *app) initCmd() *cobra.Command {
	var author, harness, zone string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a workspace: episodes.yaml, episodes/ and an empty index.yaml",
		Args:  cobra.MaximumNArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			if _, err := os.Stat(filepath.Join(dir, store.ConfigFile)); err == nil {
				return usageErr("%s already exists in %s; init never changes an existing workspace", store.ConfigFile, dir)
			}
			cfg := store.NewConfig()
			cfg.Defaults.Source.Author = firstNonEmpty(author, os.Getenv(prov.EnvAuthor))
			cfg.Defaults.Source.Harness = firstNonEmpty(harness, os.Getenv(prov.EnvHarness))
			if zone != "" {
				if _, err := temporal.LoadZone(zone); err != nil {
					return usageErr("--zone: %v", err)
				}
				cfg.Defaults.Zone = zone
			}
			ws, created, err := store.Init(dir, cfg)
			if errors.Is(err, store.ErrAlreadyExists) {
				return usageErr("%v", err)
			}
			if err != nil {
				return err
			}
			return a.emit(map[string]any{
				"workspace": map[string]any{"root": ws.Root, "format": ws.Config.Format, "id": ws.Config.Workspace.ID},
				"config":    ws.Config,
				"created":   created,
			}, func(w io.Writer) {
				fmt.Fprintf(w, "Initialised Episodes workspace at %s\n", ws.Root)
				for _, c := range created {
					fmt.Fprintf(w, "  %s\n", c)
				}
			})
		}),
	}
	cmd.Flags().StringVar(&author, "author", "", "defaults.source.author: on whose behalf episodes are recorded (URI or name; default $"+prov.EnvAuthor+")")
	cmd.Flags().StringVar(&harness, "harness", "", "defaults.source.harness (default $"+prov.EnvHarness+")")
	cmd.Flags().StringVar(&zone, "zone", "", "defaults.zone: the IANA zone written when a verb records the time itself (never taken from the host)")
	return cmd
}

func (a *app) workspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Show the resolved workspace, how it was found, and its configuration",
		Args:  cobra.NoArgs,
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			ws, err := a.openWorkspace()
			if err != nil {
				return err
			}
			out := map[string]any{"root": ws.Root, "found_by": ws.FoundBy, "config": ws.Config}
			if ws.Pointer != "" {
				out["pointer"] = ws.Pointer
			}
			return a.emit(out, func(w io.Writer) {
				fmt.Fprintf(w, "%s (found by %s)\n", ws.Root, ws.FoundBy)
				if ws.Pointer != "" {
					fmt.Fprintf(w, "  pointer: %s\n", ws.Pointer)
				}
				fmt.Fprintf(w, "  format: %s\n  workspace.id: %s\n", ws.Config.Format, ws.Config.Workspace.ID)
				if s := ws.Config.Defaults.Source.Source(); !s.IsZero() {
					fmt.Fprintf(w, "  defaults.source: %s\n", describeSource(s))
				}
				if ws.Config.Defaults.Zone != "" {
					fmt.Fprintf(w, "  defaults.zone: %s\n", ws.Config.Defaults.Zone)
				}
			})
		}),
	}
	cmd.AddCommand(a.pointerCmd())
	return cmd
}

func (a *app) pointerCmd() *cobra.Command {
	var at string
	var force bool
	cmd := &cobra.Command{
		Use:   "pointer [dir]",
		Short: "Write a .episodes pointer naming the workspace (dir, else the resolved one) in --at (default: the current directory)",
		Args:  cobra.MaximumNArgs(1),
		RunE: a.run(func(cmd *cobra.Command, args []string) error {
			var ws *store.Workspace
			var err error
			if len(args) == 1 {
				ws, err = store.Open(args[0])
			} else {
				ws, err = a.openWorkspace()
			}
			if err != nil {
				return err
			}
			if at == "" {
				if at, err = os.Getwd(); err != nil {
					return err
				}
			}
			p, err := store.WritePointer(at, ws.Root, force)
			if errors.Is(err, store.ErrAlreadyExists) {
				return refusedErr("%v", err)
			}
			if err != nil {
				return err
			}
			return a.emit(map[string]any{"pointer": p, "root": ws.Root}, func(w io.Writer) {
				fmt.Fprintf(w, "Wrote %s naming %s\n", p, ws.Root)
			})
		}),
	}
	cmd.Flags().StringVar(&at, "at", "", "directory to write the pointer in (default: the current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing pointer")
	return cmd
}
