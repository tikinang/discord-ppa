package ppa

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sort"
	"time"
)

// DebEntry represents a file to include in a .deb data archive.
type DebEntry struct {
	// Path is the absolute path inside the package (e.g. "/opt/Postman/Postman").
	Path string
	// Body is the file content. Nil for directories and symlinks.
	Body []byte
	// Mode is the file permission bits (e.g. 0755).
	Mode int64
	// IsDir marks directory entries.
	IsDir bool
	// LinkTarget is set for symlinks.
	LinkTarget string
}

// BuildDeb creates a .deb ar archive from control fields and data entries.
func BuildDeb(ctrl DebControl, entries []DebEntry) ([]byte, error) {
	controlTar, err := buildControlTar(ctrl)
	if err != nil {
		return nil, fmt.Errorf("building control.tar.gz: %w", err)
	}

	dataTar, err := buildDataTar(entries)
	if err != nil {
		return nil, fmt.Errorf("building data.tar.gz: %w", err)
	}

	var buf bytes.Buffer
	w, err := newArWriter(&buf)
	if err != nil {
		return nil, fmt.Errorf("writing ar header: %w", err)
	}

	now := time.Now()

	if err := w.writeEntry(arHeader{Name: "debian-binary", ModTime: now, Mode: 0100644}, []byte("2.0\n")); err != nil {
		return nil, err
	}
	if err := w.writeEntry(arHeader{Name: "control.tar.gz", ModTime: now, Mode: 0100644}, controlTar); err != nil {
		return nil, err
	}
	if err := w.writeEntry(arHeader{Name: "data.tar.gz", ModTime: now, Mode: 0100644}, dataTar); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildControlTar(ctrl DebControl) ([]byte, error) {
	var controlContent bytes.Buffer
	for _, f := range ctrl.Fields {
		fmt.Fprintf(&controlContent, "%s: %s\n", f.Key, f.Value)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	controlBytes := controlContent.Bytes()
	if err := tw.WriteHeader(&tar.Header{
		Name:   "./control",
		Size:   int64(len(controlBytes)),
		Mode:   0644,
		Format: tar.FormatGNU,
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(controlBytes); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildDataTar(entries []DebEntry) ([]byte, error) {
	// Sort so parent directories precede their children.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		path := "./" + e.Path
		if e.Path[0] == '/' {
			path = "." + e.Path
		}

		switch {
		case e.IsDir:
			if err := tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     path + "/",
				Mode:     e.Mode,
				Format:   tar.FormatGNU,
			}); err != nil {
				return nil, err
			}
		case e.LinkTarget != "":
			if err := tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeSymlink,
				Name:     path,
				Linkname: e.LinkTarget,
				Mode:     e.Mode,
				Format:   tar.FormatGNU,
			}); err != nil {
				return nil, err
			}
		default:
			if err := tw.WriteHeader(&tar.Header{
				Name:   path,
				Size:   int64(len(e.Body)),
				Mode:   e.Mode,
				Format: tar.FormatGNU,
			}); err != nil {
				return nil, err
			}
			if _, err := io.Copy(tw, bytes.NewReader(e.Body)); err != nil {
				return nil, err
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
