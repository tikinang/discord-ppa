package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tikinang/discord-ppa/ppa"
)

const defaultPostmanDownloadURL = "https://dl.pstmn.io/download/latest/linux64"

type PostmanSource struct {
	downloadURL string
	maintainer  string
}

func NewPostmanSource(downloadURL, maintainer string) *PostmanSource {
	if downloadURL == "" {
		downloadURL = defaultPostmanDownloadURL
	}
	return &PostmanSource{downloadURL: downloadURL, maintainer: maintainer}
}

func (p *PostmanSource) Name() string {
	return "postman"
}

func (p *PostmanSource) Description() string {
	return "Postman API development environment. Downloaded as a tar.gz from dl.pstmn.io, extracted, and repackaged into a .deb with a desktop entry and /usr/bin/postman symlink. Version is read from the embedded package.json."
}

func (p *PostmanSource) Check(ctx context.Context) (string, error) {
	resp, err := ppa.HTTPWithRetry(ctx, p.downloadURL, "HEAD")
	if err != nil {
		return "", fmt.Errorf("HEAD request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = resp.Header.Get("Content-Length")
	}
	return etag, nil
}

func (p *PostmanSource) Fetch(ctx context.Context) ([]byte, error) {
	resp, err := ppa.HTTPWithRetry(ctx, p.downloadURL, "GET")
	if err != nil {
		return nil, fmt.Errorf("downloading postman: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	tarData, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading tar.gz: %w", err)
	}

	return p.buildDeb(tarData)
}

type postmanEntry struct {
	ppa.DebEntry
}

func (p *PostmanSource) buildDeb(tarGzData []byte) ([]byte, error) {
	extracted, version, err := p.extractTarGz(tarGzData)
	if err != nil {
		return nil, fmt.Errorf("extracting tar.gz: %w", err)
	}

	if version == "" {
		return nil, fmt.Errorf("could not determine Postman version")
	}

	var entries []ppa.DebEntry

	// Collect all parent directories from extracted entries
	dirs := map[string]bool{}
	for _, e := range extracted {
		dir := filepath.Dir("/opt/" + e.Path)
		for dir != "/" && dir != "." {
			dirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}
	for dir := range dirs {
		entries = append(entries, ppa.DebEntry{
			Path:  dir,
			IsDir: true,
			Mode:  0755,
		})
	}

	// Add extracted files and symlinks under /opt/
	for _, e := range extracted {
		entry := e.DebEntry
		entry.Path = "/opt/" + e.Path
		entries = append(entries, entry)
	}

	// /usr/bin/postman symlink and desktop file
	entries = append(entries,
		ppa.DebEntry{Path: "/usr", IsDir: true, Mode: 0755},
		ppa.DebEntry{Path: "/usr/bin", IsDir: true, Mode: 0755},
		ppa.DebEntry{Path: "/usr/share", IsDir: true, Mode: 0755},
		ppa.DebEntry{Path: "/usr/share/applications", IsDir: true, Mode: 0755},
		ppa.DebEntry{
			Path:       "/usr/bin/postman",
			LinkTarget: "/opt/Postman/Postman",
			Mode:       0777,
		},
		ppa.DebEntry{
			Path: "/usr/share/applications/postman.desktop",
			Body: []byte(`[Desktop Entry]
Type=Application
Name=Postman
Comment=API Development Environment
Exec=/opt/Postman/Postman %U
Icon=/opt/Postman/app/resources/app/assets/icon.png
Terminal=false
Categories=Development;
StartupWMClass=postman
`),
			Mode: 0644,
		},
	)

	// Compute installed size in KiB
	var installedBytes int64
	for _, e := range entries {
		installedBytes += int64(len(e.Body))
	}
	installedSize := fmt.Sprintf("%d", installedBytes/1024)

	ctrl := ppa.DebControl{
		Package:      "postman",
		Version:      version,
		Architecture: "amd64",
		Maintainer:   p.maintainer,
		Description:  "Postman - API Development Environment",
		Section:      "devel",
		Priority:     "optional",
		Depends:      "libgtk-3-0, libnotify4, libnss3, libxss1, libxtst6, xdg-utils, libatspi2.0-0, libuuid1, libsecret-1-0",
		Fields: []ppa.ControlField{
			{Key: "Package", Value: "postman"},
			{Key: "Version", Value: version},
			{Key: "Architecture", Value: "amd64"},
			{Key: "Installed-Size", Value: installedSize},
			{Key: "Maintainer", Value: p.maintainer},
			{Key: "Homepage", Value: "https://www.postman.com"},
			{Key: "Depends", Value: "libgtk-3-0, libnotify4, libnss3, libxss1, libxtst6, xdg-utils, libatspi2.0-0, libuuid1, libsecret-1-0"},
			{Key: "Section", Value: "devel"},
			{Key: "Priority", Value: "optional"},
			{Key: "Description", Value: "Postman - API Development Environment\n Unofficial repackaging of the official Postman Linux build."},
		},
	}

	return ppa.BuildDeb(ctrl, entries)
}

type postmanPackageJSON struct {
	Version string `json:"version"`
}

func (p *PostmanSource) extractTarGz(data []byte) (entries []postmanEntry, version string, err error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("opening gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("reading tar: %w", err)
		}

		name := strings.TrimSuffix(hdr.Name, "/")
		if !strings.HasPrefix(name, "Postman/") || name == "Postman" {
			continue
		}

		mode := hdr.FileInfo().Mode().Perm()

		switch hdr.Typeflag {
		case tar.TypeReg:
			body, err := io.ReadAll(io.LimitReader(tr, 512*1024*1024))
			if err != nil {
				return nil, "", fmt.Errorf("reading %s: %w", name, err)
			}
			entries = append(entries, postmanEntry{
				DebEntry: ppa.DebEntry{
					Path: name,
					Body: body,
					Mode: int64(mode),
				},
			})

			if name == "Postman/app/resources/app/package.json" {
				var pkg postmanPackageJSON
				if err := json.Unmarshal(body, &pkg); err == nil && pkg.Version != "" {
					version = pkg.Version
				}
			}

		case tar.TypeSymlink:
			entries = append(entries, postmanEntry{
				DebEntry: ppa.DebEntry{
					Path:       name,
					LinkTarget: hdr.Linkname,
					Mode:       int64(mode),
				},
			})
		}
	}

	return entries, version, nil
}
