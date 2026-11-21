package mcp

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// Link is a typed link, passed to the verb as --link=rel=href.
type Link struct {
	Rel  string `json:"rel" jsonschema:"the link relation, e.g. via, predecessor-version, related"`
	Href string `json:"href" jsonschema:"an episode id or a URI"`
}

// built is a tool input turned into a verb's flags and positionals.
type built struct {
	flags []string
	pos   []string
	set   map[string]bool // flags given
}

// build turns an input struct into arguments (design D2). A field tagged
// flag:"name" becomes --name=value, each list item its own; a field tagged
// pos:"" is a positional, passed after --; path:"" resolves a relative path
// against root. Empty strings, nil pointers, false and empty lists are left
// out. Values are always joined to their flag with =, so none is read as a
// flag.
func build(in any, root string) (built, error) {
	b := built{set: map[string]bool{}}
	v := reflect.ValueOf(in)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return b, b.walk(v, root)
}

func (b *built) walk(v reflect.Value, root string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f, fv := t.Field(i), v.Field(i)
		if f.Anonymous {
			if err := b.walk(fv, root); err != nil {
				return err
			}
			continue
		}
		if _, ok := f.Tag.Lookup("pos"); ok {
			if s := strings.TrimSpace(fv.String()); s != "" {
				b.pos = append(b.pos, fv.String())
			}
			continue
		}
		name, ok := f.Tag.Lookup("flag")
		if !ok {
			continue
		}
		_, isPath := f.Tag.Lookup("path")
		add := func(val string) {
			b.flags = append(b.flags, "--"+name+"="+val)
			b.set[name] = true
		}
		switch x := fv.Interface().(type) {
		case string:
			if x == "" {
				continue
			}
			if isPath && !filepath.IsAbs(x) {
				x = filepath.Join(root, x)
			}
			add(x)
		case bool:
			if x {
				b.flags = append(b.flags, "--"+name)
				b.set[name] = true
			}
		case *int:
			if x != nil {
				add(strconv.Itoa(*x))
			}
		case []string:
			for _, s := range x {
				add(s)
			}
		case []Link:
			for _, l := range x {
				if strings.TrimSpace(l.Rel) == "" || strings.TrimSpace(l.Href) == "" {
					return fmt.Errorf("links: each link needs rel and href")
				}
				add(l.Rel + "=" + l.Href)
			}
		default:
			return fmt.Errorf("internal: field %s has unsupported type %s", f.Name, f.Type)
		}
	}
	return nil
}

// Flags lists the flag names an input type maps to, for the test that
// checks each exists on its verb.
func Flags(in any) []string {
	var out []string
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				walk(f.Type)
				continue
			}
			if n, ok := f.Tag.Lookup("flag"); ok {
				out = append(out, n)
			}
		}
	}
	t := reflect.TypeOf(in)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	walk(t)
	return out
}
