// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/sergev/dejapdf/internal/pdf"
)

// Stream is a stream object. Raw is its data as in the file, still
// encrypted if the file is; Reader.Raw and Reader.Decode undo that.
type Stream struct {
	Dict pdf.Dict
	Raw  []byte
	num  int // object number and generation, for decryption
	gen  int
}

// entry is a cross-reference entry.
type entry struct {
	kind byte // 0 free, 1 at an offset, 2 in an object stream
	off  int  // offset, or the object stream's number
	gen  int  // generation, or index in the object stream
}

// objStm is a decoded object stream.
type objStm struct {
	data []byte
	nums []int // object numbers
	offs []int // their offsets in data
}

// Reader reads objects of a PDF file held in memory. It is safe for
// concurrent use.
type Reader struct {
	data     []byte
	xref     map[int]entry
	trailer  pdf.Dict
	Repaired bool // the cross-reference data was rebuilt by scanning

	mu      sync.Mutex
	cache   map[int]any
	loading map[int]bool
	stms    map[int]*objStm
	scanned map[int]entry // objects found by scanning, for broken offsets
	crypt   *crypt
}

// ErrNotPDF means the data is not a PDF file.
var ErrNotPDF = errors.New("pdf: not a PDF file")

// Open reads the cross-reference data of a PDF file, repairing it if
// broken, and sets up decryption.
func Open(data []byte) (*Reader, error) {
	if !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-")) {
		return nil, ErrNotPDF
	}
	r := &Reader{
		data:    data,
		cache:   map[int]any{},
		loading: map[int]bool{},
		stms:    map[int]*objStm{},
	}
	err := r.readXref()
	if err == nil {
		if err := r.setCrypt(); err != nil {
			return nil, err
		}
	}
	if err != nil || !r.rootOK() {
		r.crypt = nil
		r.repair()
		if err := r.setCrypt(); err != nil {
			return nil, err
		}
		r.repairStms()
	}
	if !r.rootOK() {
		return nil, errors.New("pdf: no document catalog")
	}
	return r, nil
}

// setCrypt sets up decryption if the file is encrypted.
func (r *Reader) setCrypt() error {
	clear(r.cache)
	clear(r.stms)
	if enc := r.trailer["Encrypt"]; enc != nil {
		c, err := r.newCrypt(enc)
		if err != nil {
			return err
		}
		r.crypt = c
		clear(r.cache)
		clear(r.stms)
	}
	return nil
}

// rootOK reports whether the trailer leads to a catalog dictionary.
func (r *Reader) rootOK() bool {
	if r.trailer == nil {
		return false
	}
	_, ok := r.Resolve(r.trailer["Root"]).(pdf.Dict)
	return ok
}

// Version returns the version in the file header, such as "1.4", or
// "" if there is none.
func (r *Reader) Version() string {
	i := bytes.Index(r.data[:min(len(r.data), 1024)], []byte("%PDF-"))
	if i < 0 || i+8 > len(r.data) {
		return ""
	}
	v := r.data[i+5 : i+8]
	if v[0] < '1' || v[0] > '9' || v[1] != '.' || v[2] < '0' || v[2] > '9' {
		return ""
	}
	return string(v)
}

// Trailer returns the trailer dictionary (of the newest revision, with
// keys it lacks taken from older ones).
func (r *Reader) Trailer() pdf.Dict { return r.trailer }

// Catalog returns the document catalog.
func (r *Reader) Catalog() pdf.Dict {
	d, _ := r.Resolve(r.trailer["Root"]).(pdf.Dict)
	return d
}

// Objects returns the numbers of all objects in use, in order.
func (r *Reader) Objects() []int {
	var nums []int
	for n, e := range r.xref {
		if e.kind != 0 {
			nums = append(nums, n)
		}
	}
	slices.Sort(nums)
	return nums
}

// Resolve follows indirect references; a missing or broken object is
// null, as in viewers.
func (r *Reader) Resolve(v any) any {
	for range 32 {
		ref, ok := v.(pdf.Ref)
		if !ok {
			return v
		}
		v = r.Get(int(ref))
	}
	return nil
}

// Get returns object num, or nil.
func (r *Reader) Get(num int) any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(num)
}

func (r *Reader) get(num int) any {
	if v, ok := r.cache[num]; ok {
		return v
	}
	if r.loading[num] {
		return nil
	}
	r.loading[num] = true
	defer delete(r.loading, num)
	v := r.load(num)
	r.cache[num] = v
	return v
}

// resolve is Resolve with the lock held.
func (r *Reader) resolve(v any) any {
	for range 32 {
		ref, ok := v.(pdf.Ref)
		if !ok {
			return v
		}
		v = r.get(int(ref))
	}
	return nil
}

func (r *Reader) load(num int) any {
	e, ok := r.xref[num]
	if !ok {
		return nil
	}
	switch e.kind {
	case 1:
		v, err := r.objectAt(e.off, num)
		if err != nil {
			// Wrong offset: try where scanning finds the object.
			r.scan()
			if s, ok := r.scanned[num]; ok && s.off != e.off {
				v, err = r.objectAt(s.off, num)
			}
			if err != nil {
				return nil
			}
		}
		return v
	case 2:
		return r.fromStm(e.off, e.gen, num)
	}
	return nil
}

// objectAt parses "num G obj ... endobj" at off.
func (r *Reader) objectAt(off, num int) (any, error) {
	if off < 0 || off >= len(r.data) {
		return nil, errSyntax
	}
	l := &lexer{b: r.data, pos: off}
	n, gen, err := l.header()
	if err != nil || n != num {
		return nil, errSyntax
	}
	v, err := l.object(0)
	if err != nil {
		return nil, err
	}
	if _, ok := v.(keyword); ok {
		if v == keyword("endobj") {
			return nil, nil
		}
		return nil, errSyntax
	}
	if r.crypt != nil {
		v = r.crypt.strings(v, num, gen)
	}
	if d, ok := v.(pdf.Dict); ok {
		save := l.pos
		l.skipSpace()
		if string(l.regular()) == "stream" {
			return r.stream(l, d, num, gen), nil
		}
		l.pos = save
	}
	return v, nil
}

// header reads "N G obj".
func (l *lexer) header() (num, gen int, err error) {
	l.skipSpace()
	a := l.regular()
	l.skipSpace()
	b := l.regular()
	l.skipSpace()
	if !isDigits(a) || !isDigits(b) || len(a) == 0 || len(b) == 0 || len(a) > 10 || len(b) > 10 {
		return 0, 0, errSyntax
	}
	if string(l.regular()) != "obj" {
		return 0, 0, errSyntax
	}
	return atoi(a), atoi(b), nil
}

func atoi(b []byte) int {
	n := 0
	for _, c := range b {
		n = n*10 + int(c-'0')
		if n > 1<<40 {
			return 1 << 40
		}
	}
	return n
}

// stream reads the data after the stream keyword. /Length is trusted
// when endstream follows it; otherwise the data ends at endstream.
func (r *Reader) stream(l *lexer, d pdf.Dict, num, gen int) *Stream {
	b := r.data
	p := l.pos
	if p < len(b) && b[p] == '\r' {
		p++
	}
	if p < len(b) && b[p] == '\n' {
		p++
	}
	s := &Stream{Dict: d, num: num, gen: gen}
	if n, ok := r.resolve(d["Length"]).(int); ok && n >= 0 && n <= len(b)-p {
		e := &lexer{b: b, pos: p + n}
		e.skipSpace()
		if bytes.HasPrefix(b[e.pos:], []byte("endstream")) {
			s.Raw = b[p : p+n]
			return s
		}
	}
	end := bytes.Index(b[p:], []byte("endstream"))
	if end < 0 {
		end = len(b) - p
	}
	// The end of line before endstream is not data.
	if end > 0 && b[p+end-1] == '\n' {
		end--
	}
	if end > 0 && b[p+end-1] == '\r' {
		end--
	}
	s.Raw = b[p : p+end]
	return s
}

// fromStm returns object num at index idx of object stream sn.
func (r *Reader) fromStm(sn, idx, num int) any {
	st := r.stms[sn]
	if st == nil {
		if sn == num || r.loading[sn] {
			return nil
		}
		st = r.loadStm(sn)
		r.stms[sn] = st
	}
	i := idx
	if i < 0 || i >= len(st.nums) || st.nums[i] != num {
		// Wrong index: look the number up.
		i = -1
		for k, n := range st.nums {
			if n == num {
				i = k
			}
		}
		if i < 0 {
			return nil
		}
	}
	l := &lexer{b: st.data, pos: st.offs[i]}
	v, err := l.object(0)
	if _, kw := v.(keyword); err != nil || kw {
		return nil
	}
	return v
}

func (r *Reader) loadStm(sn int) *objStm {
	st := &objStm{}
	s, ok := r.get(sn).(*Stream)
	if !ok {
		return st
	}
	data, err := r.decode(s)
	if err != nil && len(data) == 0 {
		return st
	}
	n, _ := r.resolve(s.Dict["N"]).(int)
	first, _ := r.resolve(s.Dict["First"]).(int)
	if first < 0 || first > len(data) {
		return st
	}
	st.data = data
	l := &lexer{b: data[:first]}
	for i := 0; i < n; i++ {
		a, err1 := l.object(0)
		b, err2 := l.object(0)
		na, ok1 := a.(int)
		nb, ok2 := b.(int)
		if err1 != nil || err2 != nil || !ok1 || !ok2 || nb < 0 || first+nb > len(data) {
			break
		}
		st.nums = append(st.nums, na)
		st.offs = append(st.offs, first+nb)
	}
	return st
}

// Size is the length of the file.
func (r *Reader) Size() int64 { return int64(len(r.data)) }

// Raw returns a stream's data decrypted, still encoded.
func (r *Reader) Raw(s *Stream) []byte {
	if r.crypt == nil {
		return s.Raw
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.crypt.stream(s, r.resolve)
}

// Decode returns a stream's data decrypted and decoded.
func (r *Reader) Decode(s *Stream) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.decode(s)
}

func (r *Reader) decode(s *Stream) ([]byte, error) {
	data := s.Raw
	if r.crypt != nil {
		data = r.crypt.stream(s, r.resolve)
	}
	return decodeFilters(data, r.resolve(s.Dict["Filter"]), r.resolve(s.Dict["DecodeParms"]), r.resolve)
}

// readXref reads the cross-reference sections from startxref back
// through /Prev, newest first.
func (r *Reader) readXref() error {
	r.xref = map[int]entry{}
	i := bytes.LastIndex(r.data, []byte("startxref"))
	if i < 0 {
		return errors.New("pdf: no startxref")
	}
	l := &lexer{b: r.data, pos: i + len("startxref")}
	off, ok := mustInt(l)
	if !ok {
		return errors.New("pdf: bad startxref")
	}
	seen := map[int]bool{}
	for off != 0 {
		if seen[off] {
			break
		}
		seen[off] = true
		t, err := r.section(off)
		if err != nil {
			// Offsets may count from the %PDF header rather than byte 0.
			h := bytes.Index(r.data, []byte("%PDF-"))
			if h <= 0 || seen[off+h] {
				return err
			}
			seen[off+h] = true
			if t, err = r.section(off + h); err != nil {
				return err
			}
		}
		if r.trailer == nil {
			r.trailer = pdf.Dict{}
		}
		for k, v := range t {
			if _, ok := r.trailer[k]; !ok && k != "Prev" && k != "XRefStm" {
				r.trailer[k] = v
			}
		}
		p, _ := t["Prev"].(int)
		off = p
	}
	if r.trailer == nil {
		return errors.New("pdf: no trailer")
	}
	return nil
}

func mustInt(l *lexer) (int, bool) {
	l.skipSpace()
	tok := l.regular()
	if len(tok) == 0 || len(tok) > 15 || !isDigits(tok) {
		return 0, false
	}
	return atoi(tok), true
}

// set records an entry unless a newer section did.
func (r *Reader) set(num int, e entry) {
	if num < 0 {
		return
	}
	if _, ok := r.xref[num]; !ok {
		r.xref[num] = e
	}
}

// section reads one cross-reference table or stream at off and returns
// its trailer.
func (r *Reader) section(off int) (pdf.Dict, error) {
	if off < 0 || off >= len(r.data) {
		return nil, errors.New("pdf: xref offset out of range")
	}
	l := &lexer{b: r.data, pos: off}
	l.skipSpace()
	if !bytes.HasPrefix(r.data[l.pos:], []byte("xref")) {
		return r.xrefStream(off)
	}
	l.pos += 4
	var entries []struct {
		num int
		e   entry
	}
	for {
		l.skipSpace()
		save := l.pos
		start, ok1 := mustInt(l)
		count, ok2 := mustInt(l)
		if !ok1 || !ok2 {
			l.pos = save
			break
		}
		for i := 0; i < count; i++ {
			o, ok1 := mustInt(l)
			g, ok2 := mustInt(l)
			l.skipSpace()
			t := l.regular()
			if !ok1 || !ok2 || len(t) != 1 {
				return nil, errors.New("pdf: bad xref entry")
			}
			var e entry
			if t[0] == 'n' {
				e = entry{kind: 1, off: o, gen: g}
			} else if t[0] != 'f' {
				return nil, errors.New("pdf: bad xref entry")
			}
			entries = append(entries, struct {
				num int
				e   entry
			}{start + i, e})
		}
	}
	l.skipSpace()
	if string(l.regular()) != "trailer" {
		return nil, errors.New("pdf: no trailer")
	}
	v, err := l.object(0)
	t, ok := v.(pdf.Dict)
	if err != nil || !ok {
		return nil, errors.New("pdf: bad trailer")
	}
	// Hybrid file: the stream lists the objects that the table leaves
	// free, so it goes first (7.5.8.4).
	if x, ok := t["XRefStm"].(int); ok {
		r.xrefStream(x)
	}
	for _, e := range entries {
		// Entry 0 is the head of the free list; some writers number
		// subsections from 1 wrongly, which a check on the object fixes.
		r.set(e.num, e.e)
	}
	return t, nil
}

// xrefStream reads a cross-reference stream (7.5.8).
func (r *Reader) xrefStream(off int) (pdf.Dict, error) {
	l := &lexer{b: r.data, pos: off}
	num, _, err := l.header()
	if err != nil {
		return nil, errors.New("pdf: no xref at offset")
	}
	v, err := r.objectAt(off, num)
	s, ok := v.(*Stream)
	if err != nil || !ok {
		return nil, errors.New("pdf: bad xref stream")
	}
	data, err := decodeFilters(s.Raw, s.Dict["Filter"], s.Dict["DecodeParms"], func(v any) any { return v })
	if err != nil && len(data) == 0 {
		return nil, err
	}
	var w [3]int
	wa, _ := s.Dict["W"].(pdf.Array)
	if len(wa) < 3 {
		return nil, errors.New("pdf: bad /W")
	}
	for i := range w {
		w[i], _ = wa[i].(int)
		if w[i] < 0 || w[i] > 8 {
			return nil, errors.New("pdf: bad /W")
		}
	}
	size, _ := s.Dict["Size"].(int)
	index := pdf.Array{0, size}
	if a, ok := s.Dict["Index"].(pdf.Array); ok {
		index = a
	}
	field := func(p, n, def int) int {
		if n == 0 {
			return def
		}
		v := 0
		for i := range n {
			v = v<<8 | int(data[p+i])
		}
		return v
	}
	rec := w[0] + w[1] + w[2]
	p := 0
	for i := 0; i+1 < len(index); i += 2 {
		start, _ := index[i].(int)
		count, _ := index[i+1].(int)
		for k := 0; k < count && rec > 0 && p+rec <= len(data); k++ {
			t := field(p, w[0], 1)
			a := field(p+w[0], w[1], 0)
			b := field(p+w[0]+w[1], w[2], 0)
			p += rec
			switch t {
			case 0:
				r.set(start+k, entry{})
			case 1:
				r.set(start+k, entry{kind: 1, off: a, gen: b})
			case 2:
				r.set(start+k, entry{kind: 2, off: a, gen: b})
			}
		}
	}
	// The stream's own entry, in case the table misses it.
	r.set(num, entry{kind: 1, off: off})
	return s.Dict, nil
}

// scan finds every "N G obj" in the file; a later one wins.
func (r *Reader) scan() {
	if r.scanned != nil {
		return
	}
	r.scanned = map[int]entry{}
	b := r.data
	for i := 0; ; {
		k := bytes.Index(b[i:], []byte("obj"))
		if k < 0 {
			break
		}
		k += i
		i = k + 3
		if i < len(b) && isRegular(b[i]) || k > 0 && !isSpace(b[k-1]) {
			continue
		}
		// Back over "N G ".
		p := k
		for p > 0 && isSpace(b[p-1]) {
			p--
		}
		q := p
		for q > 0 && b[q-1] >= '0' && b[q-1] <= '9' {
			q--
		}
		if q == p || q == 0 || !isSpace(b[q-1]) {
			continue
		}
		g := q
		for g > 0 && isSpace(b[g-1]) {
			g--
		}
		s := g
		for s > 0 && b[s-1] >= '0' && b[s-1] <= '9' {
			s--
		}
		if s == g || g-s > 10 || s > 0 && isRegular(b[s-1]) {
			continue
		}
		r.scanned[atoi(b[s:g])] = entry{kind: 1, off: s, gen: atoi(b[q:p])}
	}
}

// repair rebuilds the cross-reference data by scanning: objects, the
// objects in object streams, and the last trailer (or xref stream
// dictionary) with a /Root; failing that, the catalog found by type.
func (r *Reader) repair() {
	r.Repaired = true
	r.scan()
	r.xref = map[int]entry{}
	for n, e := range r.scanned {
		r.xref[n] = e
	}
	clear(r.cache)
	clear(r.stms)
	var trailer pdf.Dict
	for i := 0; ; {
		k := bytes.Index(r.data[i:], []byte("trailer"))
		if k < 0 {
			break
		}
		l := &lexer{b: r.data, pos: i + k + len("trailer")}
		i = l.pos
		if t, ok := l.object(0); ok == nil {
			if t, ok := t.(pdf.Dict); ok && t["Root"] != nil {
				trailer = t
			}
		}
	}
	if trailer == nil {
		// The last xref stream with a /Root.
		last := -1
		for n, e := range r.scanned {
			if s, ok := r.get(n).(*Stream); ok && s.Dict["Type"] == pdf.Name("XRef") && s.Dict["Root"] != nil && e.off > last {
				trailer, last = s.Dict, e.off
			}
		}
	}
	clear(r.cache)
	if trailer == nil {
		trailer = pdf.Dict{}
	}
	r.trailer = pdf.Dict{}
	for k, v := range trailer {
		switch k {
		case "Root", "Info", "Encrypt", "ID":
			r.trailer[k] = v
		}
	}
}

// repairStms adds the objects of object streams to a repaired table,
// and finds the catalog by type if the trailer has none.
func (r *Reader) repairStms() {
	var catalog pdf.Ref
	var stms []int
	for n := range r.scanned {
		var d pdf.Dict
		switch v := r.get(n).(type) {
		case pdf.Dict:
			d = v
		case *Stream:
			d = v.Dict
			if d["Type"] == pdf.Name("ObjStm") {
				stms = append(stms, n)
			}
		}
		if d["Type"] == pdf.Name("Catalog") && d["Pages"] != nil && (catalog == 0 || n > int(catalog)) {
			catalog = pdf.Ref(n)
		}
	}
	for _, sn := range stms {
		st := r.loadStm(sn)
		r.stms[sn] = st
		for i, n := range st.nums {
			if _, ok := r.xref[n]; !ok {
				r.xref[n] = entry{kind: 2, off: sn, gen: i}
			}
		}
	}
	clear(r.cache)
	if _, ok := r.resolve(r.trailer["Root"]).(pdf.Dict); ok {
		return
	}
	if catalog == 0 {
		for n, e := range r.xref {
			if e.kind == 2 {
				if d, ok := r.get(n).(pdf.Dict); ok && d["Type"] == pdf.Name("Catalog") {
					catalog = pdf.Ref(n)
				}
			}
		}
	}
	if catalog != 0 {
		r.trailer["Root"] = catalog
	}
}

func (s *Stream) String() string { return fmt.Sprintf("stream %d %v", s.num, s.Dict) }
