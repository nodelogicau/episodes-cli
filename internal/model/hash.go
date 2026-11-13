// Origin: nodelogicau/particulars-cli internal/dkf/document.go @ e8933aacecad
// (ParseHash, HashDocumentBytes, DigestBytes and normaliseNewlines; the
// writer accepts sha256 only, as the episodes draft requires)

package model

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Registered hash algorithms. AlgorithmSHA256 is the one writers write.
const (
	AlgorithmSHA256 = "sha256"
	AlgorithmSHA512 = "sha512"
)

var (
	registeredHashes = map[string]int{AlgorithmSHA256: 64, AlgorithmSHA512: 128}
	hexDigits        = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	// hashGrammar is <algorithm>:<digest> for an algorithm this reader does
	// not compute: a name and a non-empty encoded digest.
	hashGrammar  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*:[A-Za-z0-9+/=_-]+$`)
	canonicalSHA = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ParsedHash is a content.hash as read.
type ParsedHash struct {
	Algorithm string
	Digest    string
	// Implemented is true when this reader computes the algorithm.
	Implemented bool
	// Malformed is true outside <algorithm>:<digest>, or for an implemented
	// algorithm whose digest is not hex of its length.
	Malformed bool
}

// ParseHash reads a hash. Readers accept any <algorithm>:<digest>.
func ParseHash(h string) ParsedHash {
	algo, enc, ok := strings.Cut(h, ":")
	if !ok || algo == "" || enc == "" {
		return ParsedHash{Malformed: true}
	}
	name := strings.ToLower(algo)
	switch name {
	case "sha-256":
		name = AlgorithmSHA256
	case "sha-512":
		name = AlgorithmSHA512
	}
	if n, ok := registeredHashes[name]; ok {
		p := ParsedHash{Algorithm: name, Implemented: true, Digest: strings.ToLower(enc)}
		if len(enc) != n || !hexDigits.MatchString(enc) {
			p.Malformed = true
		}
		return p
	}
	if !hashGrammar.MatchString(h) {
		return ParsedHash{Algorithm: algo, Malformed: true}
	}
	return ParsedHash{Algorithm: algo, Digest: enc}
}

// HashWriteProblem is why a writer refuses h: anything but sha256 and 64
// lowercase hex digits. A writer never normalises what it was given.
func HashWriteProblem(h string) error {
	if canonicalSHA.MatchString(h) {
		return nil
	}
	return fmt.Errorf("hash %q must be sha256:<64 lowercase hex digits>; writers write sha256 only (or use --hash-file to compute it)", h)
}

// HashBytes returns sha256 over content with every CRLF replaced by LF and
// nothing else changed: not trailing whitespace, not Unicode form, not a
// final newline.
func HashBytes(data []byte) string {
	d, _ := DigestBytes(AlgorithmSHA256, data)
	return AlgorithmSHA256 + ":" + d
}

// DigestBytes is the lowercase hex digest of content under algo, after CRLF
// normalisation, or false when this implementation does not compute algo.
func DigestBytes(algo string, data []byte) (string, bool) {
	norm := normaliseNewlines(data)
	switch algo {
	case AlgorithmSHA256:
		sum := sha256.Sum256(norm)
		return hex.EncodeToString(sum[:]), true
	case AlgorithmSHA512:
		sum := sha512.Sum512(norm)
		return hex.EncodeToString(sum[:]), true
	}
	return "", false
}

// HashMatches reports whether data hashes to h under h's algorithm. ok is
// false when the algorithm is not implemented or h is malformed.
func HashMatches(h string, data []byte) (match, ok bool) {
	p := ParseHash(h)
	if p.Malformed || !p.Implemented {
		return false, false
	}
	d, _ := DigestBytes(p.Algorithm, data)
	return d == p.Digest, true
}

func normaliseNewlines(b []byte) []byte {
	return []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
}
