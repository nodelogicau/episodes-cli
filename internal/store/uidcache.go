// The uid cache: record ref's duplicate lookup without parsing every file
// (change uid-cache, #3). It is derived, machine-local and never trusted
// over the files: it lives outside the workspace, is reconciled with the
// directory listing before every lookup, and every candidate it returns is
// for the caller to verify against its file.

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nodelogicau/episodes-cli/internal/model"
)

// EnvCacheDir overrides the base directory of derived, machine-local caches.
const EnvCacheDir = "EPISODES_CACHE_DIR"

const uidCacheVersion = "episodes-uid/1"

type uidCache struct {
	Cache     string             `json:"cache"`
	Workspace uidCacheWorkspace  `json:"workspace"`
	Dirs      map[string]*uidDir `json:"dirs"`

	path    string
	changed bool
}

type uidCacheWorkspace struct {
	ID   string `json:"id"`
	Root string `json:"root"`
}

type uidDir struct {
	Names []string `json:"names"`
	Refs  []uidRef `json:"refs,omitempty"`
}

type uidRef struct {
	ID         string `json:"id"`
	Standpoint string `json:"standpoint"`
	MediaType  string `json:"media_type"`
	UID        string `json:"uid"`
}

// absRoot is the workspace root as the cache keys it: absolute, with
// symlinks resolved where they can be.
func (w *Workspace) absRoot() string {
	abs, err := filepath.Abs(w.Root)
	if err != nil {
		return w.Root
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// uidCachePath is where this workspace's cache lives (D1), or "" when no
// cache directory can be determined.
func (w *Workspace) uidCachePath(root string) string {
	base := os.Getenv(EnvCacheDir)
	if base == "" {
		d, err := os.UserCacheDir()
		if err != nil || d == "" {
			return ""
		}
		base = d
	}
	sum := sha256.Sum256([]byte(root))
	name := w.Config.Workspace.ID + "-" + hex.EncodeToString(sum[:])[:12] + ".json"
	return filepath.Join(base, "episodes", "uid", name)
}

// loadUIDCache returns the cache, loading it once per process. A missing,
// undecodable, other-version or foreign-workspace file gives an empty cache.
func (w *Workspace) loadUIDCache() *uidCache {
	if w.uids != nil {
		return w.uids
	}
	root := w.absRoot()
	fresh := &uidCache{Cache: uidCacheVersion, Workspace: uidCacheWorkspace{ID: w.Config.Workspace.ID, Root: root},
		Dirs: map[string]*uidDir{}, path: w.uidCachePath(root), changed: true}
	w.uids = fresh
	if fresh.path == "" {
		return fresh
	}
	data, err := os.ReadFile(fresh.path)
	if err != nil {
		return fresh
	}
	var c uidCache
	if json.Unmarshal(data, &c) != nil || c.Cache != uidCacheVersion || c.Workspace != fresh.Workspace || c.Dirs == nil {
		return fresh
	}
	c.path = fresh.path
	for k, d := range c.Dirs {
		if d == nil {
			delete(c.Dirs, k)
			c.changed = true
		}
	}
	w.uids = &c
	return w.uids
}

// reconcile brings the cache up to date with the listing (D3): unseen names
// are parsed, gone names and directories dropped. A file that does not
// parse is left out, so it is tried again next time.
func (w *Workspace) reconcile(c *uidCache, l *Listing) {
	for dir := range c.Dirs {
		if _, ok := l.ByDir[dir]; !ok {
			delete(c.Dirs, dir)
			c.changed = true
		}
	}
	for dir, files := range l.ByDir {
		d := c.Dirs[dir]
		if d == nil {
			d = &uidDir{}
			c.Dirs[dir] = d
			c.changed = true
		}
		seen := make(map[string]bool, len(d.Names))
		for _, n := range d.Names {
			seen[n] = true
		}
		listed := make(map[string]bool, len(files))
		for _, f := range files {
			listed[f.ID] = true
			if seen[f.ID] {
				continue
			}
			ld := w.load(f)
			if ld.Err != nil || ld.Episode == nil {
				continue
			}
			d.add(f.ID, ld.Episode)
			seen[f.ID] = true
			c.changed = true
		}
		for n := range seen {
			if !listed[n] {
				d.keep(listed)
				c.changed = true
				break
			}
		}
	}
}

func (d *uidDir) add(name string, ep *model.Episode) {
	d.Names = append(d.Names, name)
	if ep.Content.Mode == model.ModeRef && ep.Content.UID != "" {
		d.Refs = append(d.Refs, uidRef{ID: name, Standpoint: ep.Standpoint, MediaType: lowerASCII(ep.Content.MediaType), UID: ep.Content.UID})
	}
}

func (d *uidDir) keep(listed map[string]bool) {
	names := d.Names[:0]
	for _, n := range d.Names {
		if listed[n] {
			names = append(names, n)
		}
	}
	d.Names = names
	refs := d.Refs[:0]
	for _, r := range d.Refs {
		if listed[r.ID] {
			refs = append(refs, r)
		}
	}
	d.Refs = refs
}

// UIDCandidates returns the listed files whose cached standpoint,
// media_type and uid match, after reconciling the cache with l. The caller
// verifies each against its file (D4). It never fails: when the cache cannot
// be read it is rebuilt from the files, and when it cannot be written the
// result is the same, only the next lookup is slower.
func (w *Workspace) UIDCandidates(l *Listing, standpoint, mediaType, uid string) []FileRef {
	c := w.loadUIDCache()
	w.reconcile(c, l)
	mt := lowerASCII(mediaType)
	var out []FileRef
	for dir, d := range c.Dirs {
		var hit map[string]bool
		for _, r := range d.Refs {
			if r.UID == uid && r.Standpoint == standpoint && r.MediaType == mt {
				if hit == nil {
					hit = map[string]bool{}
				}
				hit[r.ID] = true
			}
		}
		for _, f := range l.ByDir[dir] {
			if hit[f.ID] {
				out = append(out, f)
			}
		}
	}
	c.save()
	return out
}

// NoteUID adds a just-created episode to the cache, when this process has
// loaded it; otherwise the next lookup's reconcile parses the one new file.
func (w *Workspace) NoteUID(ep *model.Episode) {
	c := w.uids
	if c == nil {
		return
	}
	dir := Shard(ep.ID)
	d := c.Dirs[dir]
	if d == nil {
		d = &uidDir{}
		c.Dirs[dir] = d
	}
	for _, n := range d.Names {
		if n == ep.ID {
			return
		}
	}
	d.add(ep.ID, ep)
	c.changed = true
	c.save()
}

// save writes the cache when it changed: a temporary file renamed over it,
// without fsync, since the cache is disposable (D5). Every error is dropped.
func (c *uidCache) save() {
	if !c.changed || c.path == "" {
		return
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	dir := filepath.Dir(c.path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(c.path)+".*.tmp")
	if err != nil {
		return
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if cerr := f.Close(); werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return
	}
	if os.Rename(tmp, c.path) != nil {
		_ = os.Remove(tmp)
		return
	}
	c.changed = false
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, ch := range b {
		if 'A' <= ch && ch <= 'Z' {
			b[i] = ch + 'a' - 'A'
		}
	}
	return string(b)
}
