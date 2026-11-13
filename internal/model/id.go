// Origin: nodelogicau/particulars-cli internal/dkf/id.go @ e8933aacecad
// (NewUUID and MintedAt; the epi_ prefix replaces DKF's type prefixes)

package model

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// IDPrefix begins every episode id.
const IDPrefix = "epi_"

// idPattern is the reader's grammar: any id matching it is accepted.
var idPattern = regexp.MustCompile(`^epi_[A-Za-z0-9-]+$`)

// uuidForm is the hyphenated 36-character UUID string form, in either case.
var uuidForm = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// IsID reports whether s is an episode id under the reader's grammar.
func IsID(s string) bool { return idPattern.MatchString(s) }

// NewUUID mints a lowercase canonical UUIDv7. google/uuid's NewV7 carries a
// per-millisecond sequence, so ids minted by one process within the same
// millisecond remain lexically ordered by creation.
func NewUUID() string {
	u, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails if the random source fails; treat as fatal.
		panic(fmt.Sprintf("episodes: uuid v7: %v", err))
	}
	return strings.ToLower(u.String())
}

// NewID mints an episode id.
func NewID() string { return IDPrefix + NewUUID() }

// MintedAt returns the minting instant encoded in id's UUIDv7, in UTC, or
// false when the id carries none: an identifier part that is not a
// hyphenated UUID of version 7 and RFC 4122 variant. The instant is the
// UUID's first 48 bits, Unix milliseconds.
func MintedAt(id string) (time.Time, bool) {
	part, found := strings.CutPrefix(id, IDPrefix)
	if !found || !uuidForm.MatchString(part) {
		return time.Time{}, false
	}
	u, err := uuid.Parse(part)
	if err != nil || u.Version() != 7 || u.Variant() != uuid.RFC4122 {
		return time.Time{}, false
	}
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms).UTC(), true
}

// MonthOf returns the UTC month (YYYY-MM) of id's minting instant, the shard
// it lives under, or false when no month derives and it lives under legacy/.
func MonthOf(id string) (string, bool) {
	t, ok := MintedAt(id)
	if !ok {
		return "", false
	}
	return t.Format("2006-01"), true
}

// IsCanonicalID reports whether id is one this implementation would mint:
// epi_ and a lowercase UUIDv7.
func IsCanonicalID(id string) bool {
	_, ok := MintedAt(id)
	return ok && id == strings.ToLower(id)
}

var (
	atMu     sync.Mutex
	atLastMs int64
	atSeq    uint16
)

// NewIDAt mints an episode id whose UUIDv7 instant is t, with a monotonic
// 12-bit counter (RFC 9562 §6.2, method 1) so that ids minted by one process
// at the same instant sort in creation order. It serves --now, which tests
// use to fix the minting instant, and so the shard, as well as the
// timestamp.
func NewIDAt(t time.Time) string {
	atMu.Lock()
	ms := t.UnixMilli()
	// Within one instant the counter orders ids; a different instant, even
	// an earlier one given by a later --now, starts a fresh counter.
	if ms == atLastMs && atSeq < 0x0fff {
		atSeq++
	} else {
		var r [2]byte
		_, _ = rand.Read(r[:])
		atSeq = (uint16(r[0])<<8 | uint16(r[1])) & 0x07ff
	}
	atLastMs = ms
	seq := atSeq
	atMu.Unlock()
	var b [16]byte
	_, _ = rand.Read(b[8:])
	for i := 0; i < 6; i++ {
		b[i] = byte(uint64(ms) >> (40 - 8*i))
	}
	b[6] = 0x70 | byte(seq>>8)&0x0f
	b[7] = byte(seq)
	b[8] = 0x80 | b[8]&0x3f
	u, _ := uuid.FromBytes(b[:])
	return IDPrefix + strings.ToLower(u.String())
}
