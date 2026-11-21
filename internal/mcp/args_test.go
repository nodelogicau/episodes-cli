package mcp

import (
	"path/filepath"
	"reflect"
	"testing"
)

func intp(i int) *int { return &i }

// add-mcp-server D2.
func TestBuild(t *testing.T) {
	in := recordRefIn{
		episodeFields: episodeFields{Kind: "email", Standpoint: "ada", Participants: []string{"a", "b"},
			Links: []Link{{Rel: "via", Href: "epi_T"}}},
		placement:      placement{Ordinal: intp(2)},
		Ref:            "--kind=other",
		HashFile:       "inbox/1.eml",
		AllowDuplicate: true,
		timeFields:     timeFields{Occurred: "2026-11-16", Basis: "stated"},
	}
	b, err := build(in, "/ws")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--kind=email", "--standpoint=ada", "--participant=a", "--participant=b", "--link=via=epi_T",
		"--ordinal=2", "--ref=--kind=other", "--hash-file=" + filepath.Join("/ws", "inbox", "1.eml"), "--allow-duplicate",
		"--occurred=2026-11-16", "--basis=stated"}
	if !reflect.DeepEqual(b.flags, want) || len(b.pos) != 0 || b.set["model"] {
		t.Fatalf("%q", b.flags)
	}
	// An absolute path is kept; positionals are separate.
	abs := filepath.Join(t.TempDir(), "x")
	b, _ = build(observeIn{ID: "epi_E", ContentFile: abs}, "/ws")
	if !reflect.DeepEqual(b.flags, []string{"--content-file=" + abs}) || !reflect.DeepEqual(b.pos, []string{"epi_E"}) {
		t.Fatalf("%q %q", b.flags, b.pos)
	}
	if _, err := build(recordInlineIn{episodeFields: episodeFields{Links: []Link{{Rel: "via"}}}}, "/ws"); err == nil {
		t.Fatal("link without href")
	}
}

func TestVerbArgs(t *testing.T) {
	s := &Server{o: Options{Root: "/ws", Author: "ada", Harness: "h", Model: "m"}}
	d := &Tool{Verb: []string{"redact"}, Writes: true}
	b, _ := build(redactIn{ID: "--help", Fields: []string{"label"}, Reason: "r", modelField: modelField{Model: "x"}}, "/ws")
	got := s.verbArgs(nil, d, b)
	want := []string{"redact", "--fields=label", "--reason=r", "--model=x", "--workspace=/ws", "--json", "--author=ada", "--harness=h", "--", "--help"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	// A reader takes no attribution.
	b, _ = build(showIn{ID: "epi_1"}, "/ws")
	if got := s.verbArgs(nil, &Tool{Verb: []string{"show"}}, b); !reflect.DeepEqual(got, []string{"show", "--workspace=/ws", "--json", "--", "epi_1"}) {
		t.Fatalf("%q", got)
	}
}

func TestToolSet(t *testing.T) {
	var names []string
	for _, d := range Tools() {
		names = append(names, d.Name)
	}
	want := []string{"episode_record_inline", "episode_record_ref", "conversation_start", "conversation_turn", "act_attest",
		"episode_observe", "episode_redact", "episode_show", "episode_list", "episode_query", "episode_thread", "validate", "workspace_status"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("%v", names)
	}
}
