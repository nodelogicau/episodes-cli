// Origin: nodelogicau/particulars-cli internal/store/workspace.go @ e8933aacecad
// (Discover, openExplicit, readPointer, WritePointer, Open, Init,
// writeExclusive and writeAtomic; renamed to episodes.yaml and .episodes; the
// resolution is reported as flag, env, marker or pointer; exclusive creation
// is now atomic too)

// Package store owns everything that touches the filesystem: workspace
// discovery and episodes.yaml, the write lock, paths derived from ids, atomic
// writes, the one function that may rewrite an episode file, and the index.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nodelogicau/episodes-cli/internal/apperr"
	"github.com/nodelogicau/episodes-cli/internal/model"
)

// Workspace layout names.
const (
	EnvWorkspace = "EPISODES_WORKSPACE"
	PointerFile  = ".episodes"
	IndexFile    = "index.yaml"
	EpisodesDir  = "episodes"
	IndexDir     = "index"
	LegacyShard  = "legacy"
)

// How a workspace was found.
const (
	FoundByFlag    = "flag"
	FoundByEnv     = "env"
	FoundByMarker  = "marker"
	FoundByPointer = "pointer"
)

// Sentinel errors callers map to exit codes.
var (
	ErrNoWorkspace   = apperr.ErrNoWorkspace
	ErrNotFound      = apperr.ErrNotFound
	ErrAlreadyExists = errors.New("already exists")
)

var placeholderPath = regexp.MustCompile(`^\$\{[^}]*\}$`)

// Workspace is an opened episodes directory.
type Workspace struct {
	Root    string
	Config  Config
	FoundBy string
	// Pointer is the .episodes file followed, when FoundBy is pointer.
	Pointer string
}

// Discover locates a workspace: explicit dir, then $EPISODES_WORKSPACE, then
// an upward search from the working directory for episodes.yaml or a
// .episodes pointer. An explicit dir or the environment variable is strict:
// when it names no workspace, discovery does not fall back.
func Discover(explicit string) (*Workspace, error) {
	if explicit != "" {
		if placeholderPath.MatchString(strings.TrimSpace(explicit)) {
			return nil, fmt.Errorf("%w: %s is an unsubstituted template variable", ErrNoWorkspace, explicit)
		}
		return openExplicit(explicit, FoundByFlag)
	}
	if env := os.Getenv(EnvWorkspace); env != "" {
		return openExplicit(env, FoundByEnv)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for dir := cwd; ; {
		if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err == nil {
			return open(dir, FoundByMarker, "")
		}
		pointer := filepath.Join(dir, PointerFile)
		if target, ok, perr := readPointer(pointer); perr != nil {
			return nil, perr
		} else if ok {
			return followPointer(pointer, target)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNoWorkspace
		}
		dir = parent
	}
}

// openExplicit resolves a directory named by --workspace or the environment:
// the workspace itself, or a directory holding a .episodes pointer, one hop.
func openExplicit(dir, via string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, ConfigFile)); err == nil {
		return open(abs, via, "")
	}
	pointer := filepath.Join(abs, PointerFile)
	target, ok, perr := readPointer(pointer)
	if perr != nil {
		return nil, perr
	}
	if !ok {
		return nil, fmt.Errorf("%w (no %s or %s in %s)", ErrNoWorkspace, ConfigFile, PointerFile, abs)
	}
	return followPointer(pointer, target)
}

func followPointer(pointer, target string) (*Workspace, error) {
	if _, err := os.Stat(filepath.Join(target, ConfigFile)); err != nil {
		return nil, fmt.Errorf("%w: %s points at %s, which has no %s", ErrNoWorkspace, pointer, target, ConfigFile)
	}
	return open(target, FoundByPointer, pointer)
}

// readPointer parses a .episodes file: its first non-blank, non-comment line
// is a path, relative to the file's directory or absolute. ok is false when
// the file does not exist.
func readPointer(path string) (target string, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(filepath.Dir(path), line)
		}
		return filepath.Clean(line), true, nil
	}
	return "", false, fmt.Errorf("%w: %s is empty", ErrNoWorkspace, path)
}

// WritePointer writes <dir>/.episodes naming target. It refuses to replace
// an existing pointer with different content unless force is set.
func WritePointer(dir, target string, force bool) (string, error) {
	path := filepath.Join(dir, PointerFile)
	content := target + "\n"
	if existing, err := os.ReadFile(path); err == nil && !force {
		if string(existing) == content {
			return path, nil
		}
		return path, fmt.Errorf("%s %w (naming %q); pass --force to replace it", path, ErrAlreadyExists, strings.TrimSpace(string(existing)))
	}
	return path, writeAtomic(path, []byte(content))
}

// Open loads the workspace rooted at dir.
func Open(dir string) (*Workspace, error) { return open(dir, FoundByFlag, "") }

func open(dir, via, pointer string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := readConfig(filepath.Join(abs, ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w (no %s in %s)", ErrNoWorkspace, ConfigFile, abs)
	}
	if err != nil {
		return nil, err
	}
	return &Workspace{Root: abs, Config: cfg, FoundBy: via, Pointer: pointer}, nil
}

// Init creates a workspace at dir: episodes.yaml, episodes/ and an empty
// index.yaml. It fails if episodes.yaml already exists, and then changes
// nothing.
func Init(dir string, cfg Config) (*Workspace, []string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	cfgPath := filepath.Join(abs, ConfigFile)
	if _, err := os.Stat(cfgPath); err == nil {
		return nil, nil, fmt.Errorf("%s %w in %s", ConfigFile, ErrAlreadyExists, abs)
	}
	if err := os.MkdirAll(filepath.Join(abs, EpisodesDir), 0o755); err != nil {
		return nil, nil, err
	}
	data, err := encodeConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := writeExclusive(cfgPath, data); err != nil {
		return nil, nil, err
	}
	w := &Workspace{Root: abs, Config: cfg, FoundBy: FoundByFlag}
	if err := w.writeRoot(&Root{Format: model.Format}); err != nil {
		return nil, nil, err
	}
	return w, []string{ConfigFile, EpisodesDir + "/", IndexFile}, nil
}

// Rel returns p relative to the workspace root, slash-separated, for display.
func (w *Workspace) Rel(p string) string {
	if r, err := filepath.Rel(w.Root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

// Abs returns the absolute path of a slash-separated workspace-relative path.
func (w *Workspace) Abs(rel string) string { return filepath.Join(w.Root, filepath.FromSlash(rel)) }

// Shard is the directory under episodes/ that id derives: the UTC month of
// its UUIDv7 minting instant, or legacy when no month derives.
func Shard(id string) string {
	if m, ok := model.MonthOf(id); ok {
		return m
	}
	return LegacyShard
}

// RelPath is the workspace-relative path id derives.
func RelPath(id string) string { return EpisodesDir + "/" + Shard(id) + "/" + id + ".yaml" }

// Path is the absolute path id derives.
func (w *Workspace) Path(id string) string { return w.Abs(RelPath(id)) }

// Read loads one episode by id from the path its id derives. It never reads
// the index.
func (w *Workspace) Read(id string) (*model.Episode, []byte, error) {
	if !model.IsID(id) {
		return nil, nil, fmt.Errorf("%s: %w (not an episode id)", id, ErrNotFound)
	}
	data, err := os.ReadFile(w.Path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("episode %s: %w (no file at %s)", id, ErrNotFound, RelPath(id))
	}
	if err != nil {
		return nil, nil, err
	}
	ep, _, err := model.Decode(data)
	if err != nil {
		return nil, data, fmt.Errorf("%s: %w", RelPath(id), err)
	}
	return ep, data, nil
}

// Exists reports whether a file exists at the path id derives.
func (w *Workspace) Exists(id string) bool {
	if !model.IsID(id) {
		return false
	}
	_, err := os.Stat(w.Path(id))
	return err == nil
}

// --- low-level writes ----------------------------------------------------

// writeExclusive creates path atomically and only if it does not exist: the
// bytes go to a temporary file in the same directory, which is then linked
// into place. A link fails rather than overwrite, and either the whole file
// appears or none of it does.
func writeExclusive(path string, data []byte) error {
	tmp, err := writeTemp(path, data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s %w", filepath.Base(path), ErrAlreadyExists)
		}
		return err
	}
	return nil
}

// writeAtomic replaces path with data via a temporary file and a rename.
func writeAtomic(path string, data []byte) error {
	tmp, err := writeTemp(path, data)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeTemp(path string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// sweepTemps removes temporary files left in dir by an interrupted write.
// Every writer holds the lock, so no temporary file there is in use.
func sweepTemps(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if n := e.Name(); strings.HasPrefix(n, ".") && strings.HasSuffix(n, ".tmp") {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
}
