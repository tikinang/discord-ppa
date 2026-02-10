package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/blakesmith/ar"
)

type DebControl struct {
	Package      string
	Version      string
	Architecture string
	Maintainer   string
	Description  string
	Depends      string
	Section      string
	Priority     string
	Fields       []ControlField
}

type ControlField struct {
	Key   string
	Value string
}

func ParseDebControl(r io.Reader) (*DebControl, error) {
	arReader := ar.NewReader(r)

	for {
		header, err := arReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading ar archive: %w", err)
		}

		name := strings.TrimRight(header.Name, "/ ")

		if strings.HasPrefix(name, "control.tar") {
			return parseControlTar(arReader, name)
		}
	}

	return nil, fmt.Errorf("control.tar not found in .deb")
}

func parseControlTar(r io.Reader, name string) (*DebControl, error) {
	var tarReader *tar.Reader

	if strings.HasSuffix(name, ".gz") {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("opening gzip: %w", err)
		}
		defer gz.Close()
		tarReader = tar.NewReader(gz)
	} else if strings.HasSuffix(name, ".xz") || strings.HasSuffix(name, ".zst") {
		return nil, fmt.Errorf("%s compression not supported", name)
	} else {
		tarReader = tar.NewReader(r)
	}

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading control tar: %w", err)
		}

		cleanName := strings.TrimPrefix(hdr.Name, "./")
		if cleanName == "control" {
			return parseControlFile(tarReader)
		}
	}

	return nil, fmt.Errorf("control file not found in control.tar")
}

func parseControlFile(r io.Reader) (*DebControl, error) {
	ctrl := &DebControl{}
	scanner := bufio.NewScanner(r)

	var currentKey, currentValue string

	flush := func() {
		if currentKey == "" {
			return
		}
		value := strings.TrimSpace(currentValue)
		ctrl.Fields = append(ctrl.Fields, ControlField{Key: currentKey, Value: value})
		switch currentKey {
		case "Package":
			ctrl.Package = value
		case "Version":
			ctrl.Version = value
		case "Architecture":
			ctrl.Architecture = value
		case "Maintainer":
			ctrl.Maintainer = value
		case "Description":
			ctrl.Description = value
		case "Depends":
			ctrl.Depends = value
		case "Section":
			ctrl.Section = value
		case "Priority":
			ctrl.Priority = value
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			currentValue += "\n" + line
			continue
		}

		flush()

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		currentKey = strings.TrimSpace(parts[0])
		currentValue = strings.TrimSpace(parts[1])
	}
	flush()

	if ctrl.Package == "" {
		return nil, fmt.Errorf("Package field not found in control file")
	}

	return ctrl, nil
}
