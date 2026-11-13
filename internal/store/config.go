// Origin: nodelogicau/particulars-cli internal/store/config.go @ e8933aacecad
// (episodes.yaml in place of dkf.yaml; defaults.zone added; scope,
// base-uri and conventions dropped; the format is read before anything else)

package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/episodes-cli/internal/model"
	"github.com/nodelogicau/episodes-cli/internal/temporal"
)

// ConfigFile is the workspace marker and configuration file name.
const ConfigFile = "episodes.yaml"

// ErrUnsupportedFormat marks an episodes.yaml whose format this release does
// not implement.
var ErrUnsupportedFormat = errors.New("unsupported format")

// Config is the parsed episodes.yaml. Unknown keys are ignored on read; the
// file is only ever written by Init, so nothing is lost.
type Config struct {
	Format    string          `yaml:"format" json:"format"`
	Workspace WorkspaceConfig `yaml:"workspace" json:"workspace"`
	Defaults  Defaults        `yaml:"defaults,omitempty" json:"defaults"`
}

// WorkspaceConfig identifies the workspace.
type WorkspaceConfig struct {
	ID string `yaml:"id" json:"id"`
}

// Defaults supplies values writers use when a flag is not given.
type Defaults struct {
	Source SourceDefaults `yaml:"source,omitempty" json:"source"`
	// Zone is the IANA zone a verb writes when it supplies occurred itself.
	// It is an implementation key; the draft gives the writer no other
	// configured source of a zone (SPEC-FEEDBACK 9).
	Zone string `yaml:"zone,omitempty" json:"zone,omitempty"`
}

// SourceDefaults is defaults.source.
type SourceDefaults struct {
	Author  string `yaml:"author,omitempty" json:"author,omitempty"`
	Harness string `yaml:"harness,omitempty" json:"harness,omitempty"`
	Model   string `yaml:"model,omitempty" json:"model,omitempty"`
}

// Source returns the defaults as a model source.
func (d SourceDefaults) Source() model.Source {
	return model.Source{Author: d.Author, Harness: d.Harness, Model: d.Model}
}

// NewConfig returns a config for a fresh workspace with a minted id.
func NewConfig() Config {
	return Config{Format: model.Format, Workspace: WorkspaceConfig{ID: model.NewUUID()}}
}

// Validate checks the config is usable.
func (c Config) Validate() error {
	if c.Format != model.Format {
		return fmt.Errorf("%s: %w %q (this release implements %s)", ConfigFile, ErrUnsupportedFormat, c.Format, model.Format)
	}
	if c.Workspace.ID == "" {
		return fmt.Errorf("%s: missing workspace.id", ConfigFile)
	}
	if c.Defaults.Zone != "" {
		if _, err := temporal.LoadZone(c.Defaults.Zone); err != nil {
			return fmt.Errorf("%s: defaults.zone %v", ConfigFile, err)
		}
	}
	return nil
}

func encodeConfig(c Config) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readConfig reads episodes.yaml. The format is read first and alone: a
// workspace whose format this release does not implement is refused before
// anything else in it is read.
func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var head struct {
		Format string `yaml:"format"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return Config{}, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	if head.Format == "" {
		return Config{}, fmt.Errorf("%s: missing format", ConfigFile)
	}
	if head.Format != model.Format {
		return Config{}, fmt.Errorf("%s: %w %q (this release implements %s)", ConfigFile, ErrUnsupportedFormat, head.Format, model.Format)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return c, c.Validate()
}
