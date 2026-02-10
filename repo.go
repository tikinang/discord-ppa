package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"time"
)

type PackageInfo struct {
	Control  *DebControl
	Filename string
	Size     int64
	MD5      string
	SHA1     string
	SHA256   string
}

func GeneratePackagesFile(packages []PackageInfo) []byte {
	var buf bytes.Buffer
	for i, pkg := range packages {
		if i > 0 {
			buf.WriteString("\n")
		}
		for _, f := range pkg.Control.Fields {
			fmt.Fprintf(&buf, "%s: %s\n", f.Key, f.Value)
		}
		fmt.Fprintf(&buf, "Filename: %s\n", pkg.Filename)
		fmt.Fprintf(&buf, "Size: %d\n", pkg.Size)
		fmt.Fprintf(&buf, "MD5sum: %s\n", pkg.MD5)
		fmt.Fprintf(&buf, "SHA1: %s\n", pkg.SHA1)
		fmt.Fprintf(&buf, "SHA256: %s\n", pkg.SHA256)
		buf.WriteString("\n")
	}
	return buf.Bytes()
}

func GeneratePackagesGz(packagesData []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(packagesData); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type FileHash struct {
	Path   string
	Size   int
	MD5    string
	SHA1   string
	SHA256 string
}

func GenerateReleaseFile(files []FileHash) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Origin: ppa.matejpavlicek.cz\n")
	fmt.Fprintf(&buf, "Label: Discord PPA\n")
	fmt.Fprintf(&buf, "Suite: stable\n")
	fmt.Fprintf(&buf, "Codename: stable\n")
	fmt.Fprintf(&buf, "Architectures: amd64\n")
	fmt.Fprintf(&buf, "Components: main\n")
	fmt.Fprintf(&buf, "Date: %s\n", time.Now().UTC().Format(time.RFC1123))

	fmt.Fprintf(&buf, "MD5Sum:\n")
	for _, f := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", f.MD5, f.Size, f.Path)
	}

	fmt.Fprintf(&buf, "SHA1:\n")
	for _, f := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", f.SHA1, f.Size, f.Path)
	}

	fmt.Fprintf(&buf, "SHA256:\n")
	for _, f := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", f.SHA256, f.Size, f.Path)
	}

	return buf.Bytes()
}

func ComputeFileHash(data []byte) FileHash {
	return FileHash{
		Size:   len(data),
		MD5:    fmt.Sprintf("%x", md5.Sum(data)),
		SHA1:   fmt.Sprintf("%x", sha1.Sum(data)),
		SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
	}
}
