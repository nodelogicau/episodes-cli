// Month arithmetic and sealing.
//
// Origin: nodelogicau/particulars-cli internal/store/sharded.go @ e8933aacecad
// (monthNum, monthName, idMonth and the months type; the directory listing
// is new, since episodes have one type directory)

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

var monthDir = regexp.MustCompile(`^\d{4}-\d{2}$`)

// monthNum counts calendar months, so sealing is integer comparison.
func monthNum(t time.Time) int {
	t = t.UTC()
	return t.Year()*12 + int(t.Month()) - 1
}

// MonthName renders a month number as YYYY-MM.
func MonthName(n int) string { return fmt.Sprintf("%04d-%02d", n/12, n%12+1) }

// MonthOfName parses YYYY-MM.
func MonthOfName(s string) (int, bool) {
	if !monthDir.MatchString(s) {
		return 0, false
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return 0, false
	}
	return monthNum(t), true
}

// IDMonth is the minting month of a UUIDv7 id.
func IDMonth(id string) (int, bool) {
	t, ok := model.MintedAt(id)
	if !ok {
		return 0, false
	}
	return monthNum(t), true
}

// MonthOfTime is the month number of an instant, in UTC.
func MonthOfTime(t time.Time) int { return monthNum(t) }

// Months is the sealing state of a workspace: the newest id's month. With no
// UUIDv7 id there is no current month and nothing is sealed.
type Months struct {
	Cur int
	Has bool
}

// Open reports whether month m is open: the newest id's month, the one before
// it, or later.
func (p Months) Open(m int) bool { return !p.Has || m >= p.Cur-1 }

// See widens the state to include m.
func (p *Months) See(m int) {
	if !p.Has || m > p.Cur {
		p.Cur, p.Has = m, true
	}
}

// FileRef is one episode file found by directory listing.
type FileRef struct {
	ID string
	// Rel is the slash-separated path relative to the workspace root.
	Rel string
	// Dir is the directory under episodes/ holding it ("" for a file
	// directly in episodes/).
	Dir string
}

// Misplaced reports whether the file is not where its id derives.
func (f FileRef) Misplaced() bool { return f.Rel != RelPath(f.ID) }

// Listing is every episode file in a workspace, by directory listing alone.
type Listing struct {
	Files []FileRef
	// ByDir counts files per directory under episodes/.
	ByDir map[string][]FileRef
	// Months is the sealing state: the newest minting month among the files.
	Months Months
}

// Scan lists every *.yaml file under episodes/, without parsing any. It also
// sweeps temporary files an interrupted write left behind, when sweep is set.
func (w *Workspace) Scan() (*Listing, error) {
	root := w.Abs(EpisodesDir)
	ents, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return &Listing{ByDir: map[string][]FileRef{}}, nil
	}
	if err != nil {
		return nil, err
	}
	l := &Listing{ByDir: map[string][]FileRef{}}
	add := func(dir, name string) {
		if !strings.HasSuffix(name, ".yaml") || strings.HasPrefix(name, ".") {
			return
		}
		id := strings.TrimSuffix(name, ".yaml")
		rel := EpisodesDir + "/" + name
		if dir != "" {
			rel = EpisodesDir + "/" + dir + "/" + name
		}
		f := FileRef{ID: id, Rel: rel, Dir: dir}
		l.Files = append(l.Files, f)
		l.ByDir[dir] = append(l.ByDir[dir], f)
		if m, ok := IDMonth(id); ok {
			l.Months.See(m)
		}
	}
	for _, e := range ents {
		if !e.IsDir() {
			add("", e.Name())
			continue
		}
		sub, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, s := range sub {
			if !s.IsDir() {
				add(e.Name(), s.Name())
			}
		}
	}
	sort.Slice(l.Files, func(i, j int) bool {
		if l.Files[i].ID != l.Files[j].ID {
			return l.Files[i].ID < l.Files[j].ID
		}
		return l.Files[i].Rel < l.Files[j].Rel
	})
	return l, nil
}

// ErrSealedMonth refuses a mint into a month the workspace has sealed. Only
// a clock more than a month behind the workspace can reach it.
var ErrSealedMonth = errors.New("sealed month")

// SealedGuard refuses minting id when its month is sealed by the files.
func (l *Listing) SealedGuard(id string) error {
	m, ok := IDMonth(id)
	if !ok || l.Months.Open(m) {
		return nil
	}
	t, _ := model.MintedAt(id)
	return fmt.Errorf("%w: this machine's clock reads %s, which falls in %s, and %s is sealed because the workspace's newest id is from %s; correct the clock before recording",
		ErrSealedMonth, t.Format(time.RFC3339), MonthName(m), MonthName(m), MonthName(l.Months.Cur))
}
