package mcp

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool is one entry of the reference tool set and the verb it runs.
type Tool struct {
	Name         string
	Verb         []string
	Description  string
	Annotations  *sdk.ToolAnnotations
	Required     []string
	Writes       bool // takes attribution
	Notify       bool // notifies subscribers of its positional id after success
	Conversation bool // listed only with ConversationTools
	Input        any  // the zero input, for Flags
	add          func(s *Server, d *Tool)
}

// Tools is the full set, conversation tools included.
func Tools() []Tool { return append([]Tool(nil), toolDefs...) }

func boolp(b bool) *bool { return &b }

var (
	readOnly    = &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	additive    = &sdk.ToolAnnotations{DestructiveHint: boolp(false)}
	destructive = &sdk.ToolAnnotations{DestructiveHint: boolp(true)}
)

// def registers a tool whose input is T. The schema is T's, with Required
// set; jsonschema infers additionalProperties: false for a struct, so an
// author, harness or now in a call is refused (design D4).
func def[T any](t Tool) Tool {
	var zero T
	t.Input = zero
	t.add = func(s *Server, d *Tool) {
		schema, err := jsonschema.For[T](nil)
		if err != nil {
			panic(fmt.Errorf("%s input schema: %w", d.Name, err))
		}
		schema.Required = d.Required
		sdk.AddTool(s.srv, &sdk.Tool{Name: d.Name, Description: d.Description, Annotations: d.Annotations, InputSchema: schema},
			func(ctx context.Context, req *sdk.CallToolRequest, in T) (*sdk.CallToolResult, any, error) {
				return s.call(ctx, req, d, in), nil, nil
			})
	}
	return t
}

// --- shared parameter groups (the Reference Tool Set's three groups) -------

type episodeFields struct {
	Kind         string   `json:"kind,omitempty" flag:"kind" jsonschema:"what sort of happening: a short lowercase word (note, call, email, event, message)"`
	Standpoint   string   `json:"standpoint,omitempty" flag:"standpoint" jsonschema:"whose act, utterance or record this is: a URI where known, else a name; never defaulted"`
	Participants []string `json:"participants,omitempty" flag:"participant" jsonschema:"references to those present or addressed"`
	Place        string   `json:"place,omitempty" flag:"place" jsonschema:"a reference to where it happened"`
	Label        string   `json:"label,omitempty" flag:"label" jsonschema:"a short name captured with it, such as a subject or title; never a summary"`
	Links        []Link   `json:"links,omitempty" flag:"link" jsonschema:"typed links to earlier episodes or URIs"`
	Language     string   `json:"language,omitempty" flag:"language" jsonschema:"BCP 47 tag of the content, only when the source states it"`
}

type placement struct {
	Container string `json:"container,omitempty" flag:"container" jsonschema:"the earlier episode this one is part of"`
	Ordinal   *int   `json:"ordinal,omitempty" flag:"ordinal" jsonschema:"position within container, from 1, when order is meaningful"`
}

type timeFields struct {
	Occurred string `json:"occurred,omitempty" flag:"occurred" jsonschema:"when it happened: an RFC 3339 date-time with offset, optionally [IANA zone], or a date"`
	End      string `json:"end,omitempty" flag:"end" jsonschema:"when it ended, in the same form as occurred"`
	TZ       string `json:"tz,omitempty" flag:"tz" jsonschema:"the IANA zone a date alone is reckoned in (only with a date)"`
	Basis    string `json:"basis,omitempty" flag:"basis" jsonschema:"how the time is known: stated (a source gave it) or inferred (worked out); recorded if observed as it happened"`
}

type modelField struct {
	Model string `json:"model,omitempty" flag:"model" jsonschema:"the model behind the harness, if known"`
}

type inlineContent struct {
	Text       string `json:"text" flag:"text" jsonschema:"the text, which this workspace is the only home of"`
	MediaType  string `json:"media_type,omitempty" flag:"media-type" jsonschema:"media type of the text (default text/markdown)"`
	Derivation string `json:"derivation,omitempty" flag:"derivation" jsonschema:"verbatim, transcribed, ocr or rendered (default verbatim)"`
	Zone       string `json:"zone,omitempty" flag:"zone" jsonschema:"IANA zone written when the time is now (default: the workspace's)"`
}

// --- inputs -----------------------------------------------------------------

type recordInlineIn struct {
	episodeFields
	placement
	inlineContent
	timeFields
	modelField
}

type recordRefIn struct {
	episodeFields
	placement
	Ref            string   `json:"ref" flag:"ref" jsonschema:"where to fetch the content: a URI or an id meaningful to whatever can fetch it"`
	UID            string   `json:"uid,omitempty" flag:"uid" jsonschema:"the content's own identifier, verbatim; requires media_type"`
	MessageID      string   `json:"message_id,omitempty" flag:"message-id" jsonschema:"a mail Message-ID header value"`
	MediaType      string   `json:"media_type,omitempty" flag:"media-type" jsonschema:"media type of the content (required with uid or message_id)"`
	Version        string   `json:"version,omitempty" flag:"version" jsonschema:"the source's own version id"`
	Hash           string   `json:"hash,omitempty" flag:"hash" jsonschema:"sha256:<64 lowercase hex> of the content's bytes"`
	HashFile       string   `json:"hash_file,omitempty" flag:"hash-file" path:"" jsonschema:"a file whose bytes this server hashes; relative to the workspace root"`
	Mementos       []string `json:"mementos,omitempty" flag:"memento" jsonschema:"URIs of archived snapshots of the content"`
	AllowDuplicate bool     `json:"allow_duplicate,omitempty" flag:"allow-duplicate" jsonschema:"record although a reference episode from this standpoint has this uid (a distinct occurrence)"`
	timeFields
	modelField
}

type conversationStartIn struct {
	episodeFields
	inlineContent
	timeFields
	ContainerKind       string `json:"container_kind,omitempty" flag:"container-kind" jsonschema:"the container's kind (default conversation)"`
	ContainerStandpoint string `json:"container_standpoint,omitempty" flag:"container-standpoint" jsonschema:"whose conversation the container records (default: the bound author)"`
	modelField
}

type conversationTurnIn struct {
	Container string `json:"container" pos:"" jsonschema:"the conversation container's id"`
	episodeFields
	Ordinal *int `json:"ordinal,omitempty" flag:"ordinal" jsonschema:"the turn's position (default: one more than the highest so far)"`
	inlineContent
	timeFields
	modelField
}

type actIn struct {
	Via          string   `json:"via" flag:"via" jsonschema:"the episode, usually the turn, in which the bound author asked for the act; its standpoint must be the bound author"`
	Occurred     string   `json:"occurred" flag:"occurred" jsonschema:"when the act happened"`
	End          string   `json:"end,omitempty" flag:"end" jsonschema:"when it ended"`
	TZ           string   `json:"tz,omitempty" flag:"tz" jsonschema:"the IANA zone a date alone is reckoned in"`
	Basis        string   `json:"basis" flag:"basis" jsonschema:"stated or inferred"`
	Kind         string   `json:"kind,omitempty" flag:"kind" jsonschema:"what sort of act"`
	Participants []string `json:"participants,omitempty" flag:"participant" jsonschema:"references to those present or addressed"`
	Place        string   `json:"place,omitempty" flag:"place" jsonschema:"a reference to where it happened"`
	Label        string   `json:"label,omitempty" flag:"label" jsonschema:"a short name; never a summary"`
	Links        []Link   `json:"links,omitempty" flag:"link" jsonschema:"typed links to earlier episodes or URIs"`
	Language     string   `json:"language,omitempty" flag:"language" jsonschema:"BCP 47 tag"`
	modelField
}

type observeIn struct {
	ID          string `json:"id" pos:"" jsonschema:"the reference episode's id"`
	State       string `json:"state,omitempty" flag:"state" jsonschema:"available, changed, unreachable or gone"`
	ContentFile string `json:"content_file,omitempty" flag:"content-file" path:"" jsonschema:"the content as fetched now, hashed and compared; relative to the workspace root"`
	Version     string `json:"version,omitempty" flag:"version" jsonschema:"the source's version now"`
	modelField
}

type redactIn struct {
	ID     string   `json:"id" pos:"" jsonschema:"the episode's id"`
	Fields []string `json:"fields" flag:"fields" jsonschema:"paths to remove, from the closed set (e.g. content.text, label, content.ref)"`
	Reason string   `json:"reason" flag:"reason" jsonschema:"why; must not restate what was removed"`
	modelField
}

type showIn struct {
	ID       string `json:"id" pos:"" jsonschema:"the episode's id"`
	Children bool   `json:"children,omitempty" flag:"children" jsonschema:"also return the episodes naming it as container, in order"`
}

type filterFields struct {
	Since      string `json:"since,omitempty" flag:"since" jsonschema:"occurred on or after: a date (a whole day in your zone) or a date-time"`
	Until      string `json:"until,omitempty" flag:"until" jsonschema:"occurred on or before: a date (a whole day in your zone) or a date-time"`
	TZ         string `json:"tz,omitempty" flag:"tz" jsonschema:"the IANA zone a date alone in since or until is reckoned in (default: the workspace's, then UTC)"`
	Standpoint string `json:"standpoint,omitempty" flag:"standpoint" jsonschema:"only episodes from this standpoint"`
	Kind       string `json:"kind,omitempty" flag:"kind" jsonschema:"only episodes of this kind"`
	Container  string `json:"container,omitempty" flag:"container" jsonschema:"only episodes in this container"`
}

type listIn struct {
	filterFields
	IncludeWithdrawn bool   `json:"include_withdrawn,omitempty" flag:"include-withdrawn" jsonschema:"include inline episodes whose text was redacted"`
	By               string `json:"by,omitempty" flag:"by" jsonschema:"occurred (default), recorded (id), or ordinal (with container)"`
	Limit            *int   `json:"limit,omitempty" flag:"limit" jsonschema:"at most this many"`
}

type queryIn struct {
	Text string `json:"text" pos:"" jsonschema:"the free-text query; any term may match"`
	filterFields
	Participant string `json:"participant,omitempty" flag:"participant" jsonschema:"only episodes whose participants include this reference, as written"`
	Limit       *int   `json:"limit,omitempty" flag:"limit" jsonschema:"at most this many, after ordering"`
}

type threadIn struct {
	Container string `json:"container" pos:"" jsonschema:"the container's id"`
}

type noIn struct{}

// --- the set ----------------------------------------------------------------

var toolDefs = []Tool{
	def[recordInlineIn](Tool{Name: "episode_record_inline", Verb: []string{"record", "inline"}, Writes: true, Annotations: additive,
		Required:    []string{"kind", "standpoint", "text"},
		Description: "Record content that has no other home, such as a note. Without occurred, the time is now and basis recorded. Results equal `episodes record inline --json`."}),
	def[recordRefIn](Tool{Name: "episode_record_ref", Verb: []string{"record", "ref"}, Writes: true, Annotations: additive,
		Required:    []string{"kind", "standpoint", "ref", "occurred", "basis"},
		Description: "Record a trace of content that lives elsewhere (an email, a calendar event, a file). occurred and basis are required: the time the source states, never when you recorded it. Refused as a duplicate when the (standpoint, media_type, uid) is already recorded, naming that episode: cite it instead, or pass allow_duplicate for a distinct occurrence. Results equal `episodes record ref --json`."}),
	def[conversationStartIn](Tool{Name: "conversation_start", Verb: []string{"conversation", "start"}, Writes: true, Annotations: additive, Conversation: true,
		Required:    []string{"standpoint", "text"},
		Description: "Write a conversation container and its first turn as one act. Record turns from the harness's own transcript; do not re-type them. Results equal `episodes conversation start --json`."}),
	def[conversationTurnIn](Tool{Name: "conversation_turn", Verb: []string{"conversation", "turn"}, Writes: true, Annotations: additive, Conversation: true,
		Required:    []string{"container", "standpoint", "text"},
		Description: "Append a turn to a conversation, given the next ordinal. Record turns from the harness's own transcript; do not re-type them. Results equal `episodes conversation turn --json`."}),
	def[actIn](Tool{Name: "act_attest", Verb: []string{"act"}, Writes: true, Annotations: additive,
		Required:    []string{"via", "occurred", "basis"},
		Description: "Record a self-attested act of the bound author: something they did that left no record, recorded only on their explicit request. via names the episode (usually their turn) in which they asked; it must be in the workspace and be the bound author's. The act's standpoint is the bound author. Results equal `episodes act --via --json`."}),
	def[observeIn](Tool{Name: "episode_observe", Verb: []string{"observe"}, Writes: true, Annotations: additive, Notify: true,
		Required:    []string{"id"},
		Description: "Write source_status on a reference episode: state alone, content_file with or without version, or version alone. Results equal `episodes observe --json`."}),
	def[redactIn](Tool{Name: "episode_redact", Verb: []string{"redact"}, Writes: true, Annotations: destructive, Notify: true,
		Required:    []string{"id", "fields", "reason"},
		Description: "Remove fields from an episode and record who removed what, when and why. Only on the person's request. It cannot be undone, and version history may still hold the text. The result lists every field removed. Results equal `episodes redact --json`."}),
	def[showIn](Tool{Name: "episode_show", Verb: []string{"show"}, Annotations: readOnly,
		Required:    []string{"id"},
		Description: "One episode, located by the path its id derives; with children, the episodes it contains, in order. Results equal `episodes show --json`."}),
	def[listIn](Tool{Name: "episode_list", Verb: []string{"list"}, Annotations: readOnly,
		Description: "Episodes filtered by when they occurred, whose they are, their kind and their container, ordered by when they occurred. Withdrawn episodes only with include_withdrawn. An empty result says only that none were found. Results equal `episodes list --json`."}),
	def[queryIn](Tool{Name: "episode_query", Verb: []string{"query"}, Annotations: readOnly,
		Required:    []string{"text"},
		Description: "Episodes whose label or inline text contains any of the query's terms, under Unicode case folding, or whose content.uid equals a term exactly. No model. Each result carries resolution (uid or lexical). Never returns a withdrawn episode. Results equal `episodes query --json`."}),
	def[threadIn](Tool{Name: "episode_thread", Verb: []string{"thread"}, Annotations: readOnly,
		Required:    []string{"container"},
		Description: "A container's children in order, a withdrawn one keeping its place. Results equal `episodes thread --json`."}),
	def[noIn](Tool{Name: "validate", Verb: []string{"validate"}, Annotations: readOnly,
		Description: "Check every episode, episodes.yaml, the index and every segment. The report is the result whether or not the workspace is valid. Results equal `episodes validate --json`."}),
	def[noIn](Tool{Name: "workspace_status", Verb: []string{"workspace"}, Annotations: readOnly,
		Description: "Where this server is bound and how the workspace is configured. Results equal `episodes workspace --json`."}),
}
