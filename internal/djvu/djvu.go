// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package djvu provides the DjVu document model: pages, layers, text, outline, annotations.
package djvu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sergev/dejapdf/internal/iff"
)

// Loader reads a file that an indirect document or an INCL refers to.
type Loader func(name string) ([]byte, error)

// Document is a parsed DjVu document.
type Document struct {
	Root  *iff.Chunk
	Dir   *Dir // nil for a single-page document
	Pages []*Page

	// Warnings tell of damage the document was read past: a truncated
	// or corrupt file, a missing include.
	Warnings []error

	load  Loader
	cache map[string]*iff.Chunk // components by id
	dicts map[*iff.Chunk]*dictEntry
	mu    sync.Mutex // guards dicts and Warnings
}

// Page is one FORM:DJVU with its resolved includes. A page that cannot
// be read has Err set, and possibly no Form.
type Page struct {
	Err      error
	ID       string // component id; empty for a single-page document
	Form     *iff.Chunk
	Info     Info
	Includes []*iff.Chunk // FORM:DJVI reached through INCL, depth-first
}

// Chunks returns the data chunks with the given id, from the page
// first, then from its includes.
func (p *Page) Chunks(id string) []*iff.Chunk {
	r := p.Form.All(id)
	for _, f := range p.Includes {
		r = append(r, f.All(id)...)
	}
	return r
}

// Open reads a document; referenced files are looked up next to it.
func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	return Parse(data, func(name string) ([]byte, error) {
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("djvu: refusing non-local file name %q", name)
		}
		return os.ReadFile(filepath.Join(dir, name))
	})
}

// Parse parses a document from memory. load may be nil if the
// document references no other files.
func Parse(data []byte, load Loader) (*Document, error) {
	root, err := iff.Parse(data)
	if root == nil {
		return nil, err
	}
	d := &Document{Root: root, load: load, cache: map[string]*iff.Chunk{}}
	if err != nil {
		d.Warnings = append(d.Warnings, err)
	}
	switch root.Name() {
	case "FORM:DJVU":
		p, err := d.newPage("", root)
		if p == nil {
			return nil, err
		}
		d.Pages = []*Page{p}
	case "FORM:DJVM":
		c := root.Find("DIRM")
		if c == nil {
			return nil, fmt.Errorf("djvu: FORM:DJVM without DIRM")
		}
		dir, err := ParseDir(c.Data)
		if err != nil {
			return nil, err
		}
		if err := d.build(dir); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("djvu: unsupported document type %s", root.Name())
	}
	return d, nil
}

// build creates pages from a decoded directory.
func (d *Document) build(dir *Dir) error {
	d.Dir = dir
	for _, c := range dir.Components {
		if c.Type != PageComp {
			continue
		}
		f, err := d.component(c)
		if err == nil && f.Name() != "FORM:DJVU" {
			err = fmt.Errorf("djvu: page %q is %s", c.ID, f.Name())
		}
		if err != nil {
			d.Pages = append(d.Pages, &Page{ID: c.ID, Err: err})
			continue
		}
		p, err := d.newPage(c.ID, f)
		if p == nil {
			p = &Page{ID: c.ID, Err: err}
		}
		d.Pages = append(d.Pages, p)
	}
	if len(d.Pages) == 0 {
		return errors.New("djvu: no pages")
	}
	return nil
}

// component returns the FORM of a directory entry.
func (d *Document) component(c Component) (*iff.Chunk, error) {
	if f, ok := d.cache[c.ID]; ok {
		return f, nil
	}
	var f *iff.Chunk
	if d.Dir.Bundled {
		for _, k := range d.Root.Children {
			if k.Offset == c.Offset {
				f = k
				break
			}
		}
		if f == nil {
			return nil, fmt.Errorf("djvu: component %q: no chunk at offset %d", c.ID, c.Offset)
		}
	} else {
		var err error
		if f, err = d.loadFile(c.Name); err != nil {
			return nil, err
		}
	}
	d.cache[c.ID] = f
	return f, nil
}

func (d *Document) loadFile(name string) (*iff.Chunk, error) {
	if d.load == nil {
		return nil, fmt.Errorf("djvu: cannot load %q: no loader", name)
	}
	data, err := d.load(name)
	if err != nil {
		return nil, err
	}
	f, err := iff.Parse(data)
	if err != nil {
		err = fmt.Errorf("djvu: %s: %w", name, err)
		if f == nil {
			return nil, err
		}
		d.warn(err)
	}
	return f, nil
}

// lookup resolves an INCL id.
func (d *Document) lookup(id string) (*iff.Chunk, error) {
	if d.Dir != nil {
		for _, c := range d.Dir.Components {
			if c.ID == id {
				return d.component(c)
			}
		}
		return nil, fmt.Errorf("djvu: included component %q not in directory", id)
	}
	// Single-page file: ids are file names next to it.
	if f, ok := d.cache[id]; ok {
		return f, nil
	}
	f, err := d.loadFile(id)
	if err != nil {
		return nil, err
	}
	d.cache[id] = f
	return f, nil
}

// newPage reads a page. It returns nil and an error if the page has no
// usable INFO; an include that fails is a warning.
func (d *Document) newPage(id string, f *iff.Chunk) (*Page, error) {
	c := f.Find("INFO")
	if c == nil {
		return nil, fmt.Errorf("djvu: page %q has no INFO", id)
	}
	in, err := ParseInfo(c.Data)
	if err != nil {
		return nil, err
	}
	p := &Page{ID: id, Form: f, Info: in}
	seen := map[*iff.Chunk]bool{f: true}
	var walk func(*iff.Chunk)
	walk = func(f *iff.Chunk) {
		for _, k := range f.All("INCL") {
			t, err := d.lookup(strings.TrimSpace(string(k.Data)))
			if err == nil && t.Name() != "FORM:DJVI" {
				err = fmt.Errorf("djvu: INCL %q is %s", k.Data, t.Name())
			}
			if err != nil {
				d.warn(fmt.Errorf("page %q: %w", id, err))
				continue
			}
			if seen[t] {
				continue
			}
			seen[t] = true
			p.Includes = append(p.Includes, t)
			walk(t)
		}
	}
	walk(f)
	return p, nil
}

// warn records damage, once per message.
func (d *Document) warn(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.Warnings {
		if w.Error() == err.Error() {
			return
		}
	}
	d.Warnings = append(d.Warnings, err)
}
