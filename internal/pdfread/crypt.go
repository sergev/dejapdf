// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"hash"

	"github.com/sergev/dejapdf/internal/pdf"
)

// ErrPassword means the file needs a user password to open.
var ErrPassword = errors.New("pdf: file is password-protected")

// Crypt filter methods.
const (
	cfNone = iota
	cfRC4
	cfAESV2
	cfAESV3
)

// crypt is the standard security handler (7.6.4) opened with the empty
// user password.
type crypt struct {
	key     []byte
	stm     int // method for streams
	str     int // and for strings
	filters map[string]int
	encMeta bool
	encNum  int // the Encrypt dictionary, whose strings are plain
	r       int // revision
}

// padding is the password padding string (7.6.4.3.2).
var padding = []byte("\x28\xbf\x4e\x5e\x4e\x75\x8a\x41\x64\x00\x4e\x56\xff\xfa\x01\x08" +
	"\x2e\x2e\x00\xb6\xd0\x68\x3e\x80\x2f\x0c\xa9\xfe\x64\x53\x69\x7a")

func (r *Reader) newCrypt(enc any) (*crypt, error) {
	c := &crypt{encMeta: true, filters: map[string]int{"Identity": cfNone}}
	if ref, ok := enc.(pdf.Ref); ok {
		c.encNum = int(ref)
	} else {
		c.encNum = -1
	}
	d, ok := r.Resolve(enc).(pdf.Dict)
	if !ok {
		return nil, errors.New("pdf: bad /Encrypt")
	}
	if d["Filter"] != pdf.Name("Standard") {
		return nil, errors.New("pdf: unsupported security handler")
	}
	num := func(k string) int { n, _ := r.Resolve(d[k]).(int); return n }
	str := func(k string) []byte { s, _ := r.Resolve(d[k]).(pdf.String); return []byte(s) }
	v, rev := num("V"), num("R")
	c.r = rev
	if b, ok := r.Resolve(d["EncryptMetadata"]).(bool); ok {
		c.encMeta = b
	}
	n := 5 // key length in bytes
	if v >= 2 {
		if l := num("Length"); l >= 40 && l <= 128 {
			n = l / 8
		}
	}
	switch v {
	case 1, 2:
		c.stm, c.str = cfRC4, cfRC4
	case 4, 5:
		n = 16
		cf, _ := r.Resolve(d["CF"]).(pdf.Dict)
		for name, f := range cf {
			fd, _ := r.Resolve(f).(pdf.Dict)
			m := cfNone
			switch r.Resolve(fd["CFM"]) {
			case pdf.Name("V2"):
				m = cfRC4
				if l, ok := r.Resolve(fd["Length"]).(int); ok && l >= 5 && l <= 16 {
					n = l
				} else if l >= 40 && l <= 128 {
					n = l / 8
				}
			case pdf.Name("AESV2"):
				m = cfAESV2
			case pdf.Name("AESV3"):
				m = cfAESV3
			}
			c.filters[name] = m
		}
		pick := func(k string) int {
			name, ok := r.Resolve(d[k]).(pdf.Name)
			if !ok {
				return cfNone
			}
			return c.filters[string(name)]
		}
		c.stm, c.str = pick("StmF"), pick("StrF")
	default:
		return nil, errors.New("pdf: unsupported encryption")
	}
	o, u := str("O"), str("U")
	var id []byte
	if a, ok := r.Resolve(r.trailer["ID"]).(pdf.Array); ok && len(a) > 0 {
		s, _ := r.Resolve(a[0]).(pdf.String)
		id = []byte(s)
	}
	switch {
	case rev >= 2 && rev <= 4:
		if len(o) < 32 || len(u) < 32 {
			return nil, errors.New("pdf: bad /O or /U")
		}
		c.key = c.key24(o, uint32(num("P")), id, n)
		cmp := 16
		if rev == 2 {
			cmp = 32
		}
		if !bytes.Equal(c.user24(id)[:cmp], u[:cmp]) {
			return nil, ErrPassword
		}
	case rev == 5 || rev == 6:
		ue := str("UE")
		if len(u) < 48 || len(ue) < 32 {
			return nil, errors.New("pdf: bad /U or /UE")
		}
		if !bytes.Equal(hash6(rev, nil, u[32:40], nil), u[:32]) {
			return nil, ErrPassword
		}
		k := hash6(rev, nil, u[40:48], nil)
		b, _ := aes.NewCipher(k)
		c.key = make([]byte, 32)
		cipher.NewCBCDecrypter(b, make([]byte, 16)).CryptBlocks(c.key, ue[:32])
	default:
		return nil, errors.New("pdf: unsupported encryption revision")
	}
	return c, nil
}

// key24 computes the file key for revisions 2-4 (algorithm 2).
func (c *crypt) key24(o []byte, p uint32, id []byte, n int) []byte {
	h := md5.New()
	h.Write(padding)
	h.Write(o[:32])
	binary.Write(h, binary.LittleEndian, p)
	h.Write(id)
	if c.r >= 4 && !c.encMeta {
		h.Write([]byte{0xff, 0xff, 0xff, 0xff})
	}
	key := h.Sum(nil)[:n]
	if c.r >= 3 {
		for range 50 {
			s := md5.Sum(key)
			key = s[:n]
		}
	}
	return key
}

// user24 computes /U for the empty password (algorithms 4, 5).
func (c *crypt) user24(id []byte) []byte {
	if c.r == 2 {
		out := make([]byte, 32)
		rc, _ := rc4.NewCipher(c.key)
		rc.XORKeyStream(out, padding)
		return out
	}
	h := md5.New()
	h.Write(padding)
	h.Write(id)
	out := h.Sum(nil)
	k := make([]byte, len(c.key))
	for i := range 20 {
		for j := range k {
			k[j] = c.key[j] ^ byte(i)
		}
		rc, _ := rc4.NewCipher(k)
		rc.XORKeyStream(out, out)
	}
	return out
}

// hash6 is the password hash of revision 5 (SHA-256) and 6
// (algorithm 2.B).
func hash6(rev int, pw, salt, udata []byte) []byte {
	h := sha256.New()
	h.Write(pw)
	h.Write(salt)
	h.Write(udata)
	k := h.Sum(nil)
	if rev == 5 {
		return k
	}
	for round := 1; ; round++ {
		k1 := bytes.Repeat(append(append(append([]byte{}, pw...), k...), udata...), 64)
		b, _ := aes.NewCipher(k[:16])
		e := make([]byte, len(k1))
		cipher.NewCBCEncrypter(b, k[16:32]).CryptBlocks(e, k1)
		sum := 0
		for _, x := range e[:16] {
			sum += int(x)
		}
		var hh hash.Hash
		switch sum % 3 {
		case 0:
			hh = sha256.New()
		case 1:
			hh = sha512.New384()
		default:
			hh = sha512.New()
		}
		hh.Write(e)
		k = hh.Sum(nil)
		if round >= 64 && int(e[len(e)-1]) <= round-32 {
			return k[:32]
		}
	}
}

// objKey is the key for one object (algorithm 1).
func (c *crypt) objKey(m, num, gen int) []byte {
	if m == cfAESV3 {
		return c.key
	}
	h := md5.New()
	h.Write(c.key)
	h.Write([]byte{byte(num), byte(num >> 8), byte(num >> 16), byte(gen), byte(gen >> 8)})
	if m == cfAESV2 {
		h.Write([]byte("sAlT"))
	}
	return h.Sum(nil)[:min(len(c.key)+5, 16)]
}

func (c *crypt) decrypt(m int, data []byte, num, gen int) []byte {
	switch m {
	case cfRC4:
		out := make([]byte, len(data))
		rc, _ := rc4.NewCipher(c.objKey(m, num, gen))
		rc.XORKeyStream(out, data)
		return out
	case cfAESV2, cfAESV3:
		if len(data) < 32 {
			return nil
		}
		b, _ := aes.NewCipher(c.objKey(m, num, gen))
		out := make([]byte, (len(data)-16)/16*16)
		cipher.NewCBCDecrypter(b, data[:16]).CryptBlocks(out, data[16:16+len(out)])
		// PKCS#5 padding; left as is if broken.
		if p := int(out[len(out)-1]); p >= 1 && p <= 16 {
			out = out[:len(out)-p]
		}
		return out
	}
	return data
}

// strings decrypts the strings in v, an object read from the file.
func (c *crypt) strings(v any, num, gen int) any {
	if num == c.encNum || c.str == cfNone {
		return v
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch v := v.(type) {
		case pdf.String:
			return pdf.String(c.decrypt(c.str, []byte(v), num, gen))
		case pdf.Array:
			for i := range v {
				v[i] = walk(v[i])
			}
		case pdf.Dict:
			for k := range v {
				v[k] = walk(v[k])
			}
		}
		return v
	}
	return walk(v)
}

// stream decrypts a stream's data.
func (c *crypt) stream(s *Stream, resolve func(any) any) []byte {
	t := s.Dict["Type"]
	if t == pdf.Name("XRef") || t == pdf.Name("Metadata") && !c.encMeta {
		return s.Raw
	}
	m := c.stm
	// A /Crypt filter names its own crypt filter (7.6.6).
	f := resolve(s.Dict["Filter"])
	if a, ok := f.(pdf.Array); ok && len(a) > 0 {
		f = a[0]
	}
	if f == pdf.Name("Crypt") {
		m = cfNone
		p := resolve(s.Dict["DecodeParms"])
		if a, ok := p.(pdf.Array); ok && len(a) > 0 {
			p = resolve(a[0])
		}
		if d, ok := p.(pdf.Dict); ok {
			if name, ok := d["Name"].(pdf.Name); ok {
				m = c.filters[string(name)]
			}
		}
	}
	return c.decrypt(m, s.Raw, s.num, s.gen)
}
