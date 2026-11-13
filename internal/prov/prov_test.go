package prov

import (
	"testing"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

func TestResolveOrder(t *testing.T) {
	def := model.Source{Author: "https://example.com/ada", Harness: "def-harness"}
	t.Setenv(EnvHarness, "env-harness")
	t.Setenv(EnvModel, "${user_config.model}")
	got := Resolve(def, Explicit{Harness: "claude"})
	if got.Author != "https://example.com/ada" || got.Harness != "claude" || got.Model != "" {
		t.Fatalf("%+v", got)
	}
	got = Resolve(def, Explicit{})
	if got.Harness != "env-harness" {
		t.Fatalf("env should beat defaults: %+v", got)
	}
	if Require(model.Source{Model: "m"}) == nil || Require(model.Source{Harness: "h"}) != nil {
		t.Fatal("Require")
	}
	if got := Resolve(model.Source{}, Explicit{Author: "${x}"}); got.Author != "" {
		t.Fatal("placeholder kept")
	}
}
