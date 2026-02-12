package ppa

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	arGlobalHeader = "!<arch>\n"
	arHeaderSize   = 60
)

type arHeader struct {
	Name    string
	ModTime time.Time
	Size    int64
	Mode    int64
}

// arReader reads entries from an ar archive.
type arReader struct {
	r         io.Reader
	remaining int64 // bytes left in current entry
	pad       int64 // padding byte after current entry (0 or 1)
}

func newArReader(r io.Reader) (*arReader, error) {
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("reading ar global header: %w", err)
	}
	if string(magic[:]) != arGlobalHeader {
		return nil, fmt.Errorf("not an ar archive")
	}
	return &arReader{r: r}, nil
}

func (ar *arReader) next() (*arHeader, error) {
	// Skip unread data + padding from previous entry
	skip := ar.remaining + ar.pad
	if skip > 0 {
		if _, err := io.CopyN(io.Discard, ar.r, skip); err != nil {
			return nil, err
		}
	}
	ar.remaining = 0
	ar.pad = 0

	var buf [arHeaderSize]byte
	if _, err := io.ReadFull(ar.r, buf[:]); err != nil {
		return nil, err // io.EOF or io.ErrUnexpectedEOF
	}

	// Validate magic bytes at offset 58-59
	if buf[58] != '`' || buf[59] != '\n' {
		return nil, fmt.Errorf("invalid ar entry header magic")
	}

	name := strings.TrimRight(string(buf[0:16]), " ")
	size, err := strconv.ParseInt(strings.TrimSpace(string(buf[48:58])), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing ar entry size: %w", err)
	}

	var modTime int64
	if s := strings.TrimSpace(string(buf[16:28])); s != "" {
		modTime, _ = strconv.ParseInt(s, 10, 64)
	}

	var mode int64
	if s := strings.TrimSpace(string(buf[40:48])); s != "" {
		mode, _ = strconv.ParseInt(s, 8, 64)
	}

	ar.remaining = size
	if size%2 == 1 {
		ar.pad = 1
	}

	return &arHeader{
		Name:    name,
		ModTime: time.Unix(modTime, 0),
		Size:    size,
		Mode:    mode,
	}, nil
}

// Read reads from the current entry's data.
func (ar *arReader) Read(b []byte) (int, error) {
	if ar.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(b)) > ar.remaining {
		b = b[:ar.remaining]
	}
	n, err := ar.r.Read(b)
	ar.remaining -= int64(n)
	return n, err
}

// arWriter writes an ar archive.
type arWriter struct {
	w io.Writer
}

func newArWriter(w io.Writer) (*arWriter, error) {
	if _, err := io.WriteString(w, arGlobalHeader); err != nil {
		return nil, err
	}
	return &arWriter{w: w}, nil
}

// writeEntry writes a complete ar entry (header + data + padding).
func (aw *arWriter) writeEntry(h arHeader, data []byte) error {
	var buf [arHeaderSize]byte
	for i := range buf {
		buf[i] = ' '
	}

	copy(buf[0:16], fmt.Sprintf("%-16s", h.Name))
	copy(buf[16:28], fmt.Sprintf("%-12d", h.ModTime.Unix()))
	copy(buf[28:34], fmt.Sprintf("%-6d", 0))  // uid
	copy(buf[34:40], fmt.Sprintf("%-6d", 0))  // gid
	copy(buf[40:48], fmt.Sprintf("%-8o", h.Mode))
	copy(buf[48:58], fmt.Sprintf("%-10d", len(data)))
	buf[58] = '`'
	buf[59] = '\n'

	if _, err := aw.w.Write(buf[:]); err != nil {
		return err
	}
	if _, err := aw.w.Write(data); err != nil {
		return err
	}
	// Pad to even boundary
	if len(data)%2 == 1 {
		if _, err := aw.w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}
