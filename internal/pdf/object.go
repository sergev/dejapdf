// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdf

import (
	"fmt"
	"slices"
	"strconv"
	"unicode/utf16"
)

// Object values: bool, int, float64, Name, String, Array, Dict, Ref,
// Raw, nil (null).

// Name is a PDF name, written with a leading slash.
type Name string

// String is a byte string, written literally or in hex.
type String string

// Array is a PDF array.
type Array []any

// Dict is a PDF dictionary; nil values are left out. Keys are written
// sorted, so output is deterministic.
type Dict map[string]any

// Ref is an indirect reference; 0 is no object.
type Ref int

// Raw is written as is.
type Raw string

// TextString encodes text for a text string (7.9.2.2): as is if
// printable ASCII, otherwise UTF-16BE with a byte order mark.
func TextString(s string) String {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		return String(s)
	}
	b := []byte{0xfe, 0xff}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u>>8), byte(u))
	}
	return String(b)
}

// appendObject appends the PDF syntax of v.
func appendObject(b []byte, v any) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, "null"...)
	case bool:
		return strconv.AppendBool(b, v)
	case int:
		return strconv.AppendInt(b, int64(v), 10)
	case float64:
		return AppendReal(b, v)
	case Name:
		return appendName(b, v)
	case String:
		return appendString(b, v)
	case Ref:
		return fmt.Appendf(b, "%d 0 R", int(v))
	case Raw:
		return append(b, v...)
	case Array:
		b = append(b, '[')
		for i, e := range v {
			n := len(b)
			b = appendObject(b, e)
			if i > 0 && needSpace(b[n-1], b[n]) {
				b = slices.Insert(b, n, ' ')
			}
		}
		return append(b, ']')
	case Dict:
		keys := make([]string, 0, len(v))
		for k, e := range v {
			if e != nil {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		b = append(b, "<<"...)
		for _, k := range keys {
			b = appendName(b, Name(k))
			n := len(b)
			b = appendObject(b, v[k])
			if needSpace(b[n-1], b[n]) {
				b = slices.Insert(b, n, ' ')
			}
		}
		return append(b, ">>"...)
	}
	panic(fmt.Sprintf("pdf: unsupported value %T", v))
}

// needSpace reports whether tokens ending in a and starting with b must
// be separated: delimiters end a token by themselves (7.2.2).
func needSpace(a, b byte) bool { return isRegular(a) && isRegular(b) }

// AppendReal appends a number with at most 4 decimals, no exponent.
func AppendReal(b []byte, f float64) []byte {
	s := strconv.FormatFloat(f, 'f', 4, 64)
	for s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "-0" {
		s = "0"
	}
	return append(b, s...)
}

func isRegular(c byte) bool {
	if c <= ' ' || c > '~' || c == '#' {
		return false
	}
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

func appendName(b []byte, n Name) []byte {
	b = append(b, '/')
	for i := 0; i < len(n); i++ {
		if c := n[i]; isRegular(c) {
			b = append(b, c)
		} else {
			b = fmt.Appendf(b, "#%02X", c)
		}
	}
	return b
}

// appendString writes printable text literally, anything else in hex.
func appendString(b []byte, s String) []byte {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return fmt.Appendf(b, "<%X>", []byte(s))
		}
	}
	b = append(b, '(')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '(', ')', '\\':
			b = append(b, '\\', c)
		default:
			b = append(b, c)
		}
	}
	return append(b, ')')
}
