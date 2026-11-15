// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package pdfread reads PDF files: objects, cross-reference tables and
// streams, object streams, incremental updates, the standard security
// handler and the page tree. A damaged file is repaired by scanning it
// for objects. Values use the types of package pdf, plus *Stream.
package pdfread
