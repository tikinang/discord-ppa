package ppa

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxDebSize = 512 * 1024 * 1024 // 512 MB

var safeDebField = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+~:\-]*$`)

type Config struct {
	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3Region    string

	GPGPrivateKey string

	ListenAddr string

	Origin     string // e.g. "ppa.matejpavlicek.cz"
	Label      string // e.g. "PPA"
	Maintainer string // e.g. "PPA <ppa@example.com>"
}

type SourceRegistration struct {
	Source       Source
	PollInterval time.Duration
}

type PPA struct {
	cfg    Config
	s3     *S3Client
	signer *GPGSigner
	mu     sync.Mutex // serializes repo metadata regeneration

	sources []SourceRegistration
}

func New(cfg Config) (*PPA, error) {
	signer, err := NewGPGSigner(cfg.GPGPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("GPG error: %w", err)
	}

	s3Client := NewS3Client(S3Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
	})

	return &PPA{
		cfg:    cfg,
		s3:     s3Client,
		signer: signer,
	}, nil
}

func (p *PPA) Register(reg SourceRegistration) {
	p.sources = append(p.sources, reg)
}

// DeleteSource removes all pool files, metadata, and state for a source,
// then regenerates repo metadata.
func (p *PPA) DeleteSource(ctx context.Context, sourceName string) error {
	slog.Info("Deleting source", "source", sourceName)

	// Find and delete all pool files referenced by this source's packages-entry
	entryData, err := p.s3.Download(ctx, "meta/"+sourceName+"/packages-entry")
	if err == nil {
		for _, line := range strings.Split(string(entryData), "\n") {
			if strings.HasPrefix(line, "Filename: ") {
				filename := strings.TrimPrefix(line, "Filename: ")
				slog.Info("Deleting file", "source", sourceName, "file", filename)
				if err := p.s3.Delete(ctx, filename); err != nil {
					slog.Warn("Failed to delete file", "source", sourceName, "file", filename, "error", err)
				}
			}
		}
	}

	// Delete meta files
	for _, key := range []string{
		"meta/" + sourceName + "/packages-entry",
		"meta/" + sourceName + "/state",
	} {
		slog.Info("Deleting meta", "source", sourceName, "key", key)
		if err := p.s3.Delete(ctx, key); err != nil {
			slog.Warn("Failed to delete meta", "source", sourceName, "key", key, "error", err)
		}
	}

	// Regenerate repo metadata without this source
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.regenerateRepoMetadata(ctx); err != nil {
		return fmt.Errorf("regenerating repo metadata: %w", err)
	}

	slog.Info("Source deleted successfully", "source", sourceName)
	return nil
}

func (p *PPA) Run(ctx context.Context) error {
	var sources []sourceInfo
	for _, reg := range p.sources {
		sources = append(sources, sourceInfo{
			Name:        reg.Source.Name(),
			Description: reg.Source.Description(),
		})
	}

	srv := newServer(p.s3, p.signer, sources, p.cfg.Maintainer)
	server := &http.Server{
		Addr:         p.cfg.ListenAddr,
		Handler:      srv.handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Minute,
	}

	var wg sync.WaitGroup

	for _, reg := range p.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.runPoller(ctx, reg)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info("Listening", "addr", p.cfg.ListenAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	wg.Wait()
	slog.Info("Shutdown complete")
	return nil
}

func (p *PPA) runPoller(ctx context.Context, reg SourceRegistration) {
	name := reg.Source.Name()
	slog.Info("Starting poller", "source", name, "interval", reg.PollInterval)

	p.poll(ctx, reg)

	ticker := time.NewTicker(reg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx, reg)
		}
	}
}

func (p *PPA) poll(ctx context.Context, reg SourceRegistration) {
	name := reg.Source.Name()
	slog.Debug("Polling for new version", "source", name)

	state, err := reg.Source.Check(ctx)
	if err != nil {
		slog.Error("Check failed", "source", name, "error", err)
		return
	}

	lastState, err := p.s3.Download(ctx, "meta/"+name+"/state")
	if err == nil && string(lastState) == state && state != "" {
		slog.Debug("No new version detected", "source", name)
		return
	}

	slog.Info("New version detected, fetching", "source", name)

	debData, err := reg.Source.Fetch(ctx)
	if err != nil {
		slog.Error("Fetch failed", "source", name, "error", err)
		return
	}

	if err := p.processNewDeb(ctx, name, state, debData); err != nil {
		slog.Error("Error processing new version", "source", name, "error", err)
	}
}

func (p *PPA) processNewDeb(ctx context.Context, sourceName, state string, debData []byte) error {
	if len(debData) > maxDebSize {
		return fmt.Errorf(".deb exceeds maximum size (%d bytes)", maxDebSize)
	}

	ctrl, err := ParseDebControl(bytes.NewReader(debData))
	if err != nil {
		return fmt.Errorf("parsing .deb: %w", err)
	}

	if !safeDebField.MatchString(ctrl.Package) || !safeDebField.MatchString(ctrl.Version) {
		return fmt.Errorf("invalid package name %q or version %q", ctrl.Package, ctrl.Version)
	}

	firstLetter := string(ctrl.Package[0])
	filename := fmt.Sprintf("pool/%s/%s/%s-%s.deb", firstLetter, ctrl.Package, ctrl.Package, ctrl.Version)

	md5sum := fmt.Sprintf("%x", md5.Sum(debData))
	sha1sum := fmt.Sprintf("%x", sha1.Sum(debData))
	sha256sum := fmt.Sprintf("%x", sha256.Sum256(debData))

	slog.Info("Uploading package", "source", sourceName, "file", filename, "bytes", len(debData))
	if err := p.s3.Upload(ctx, filename, debData, "application/vnd.debian.binary-package"); err != nil {
		return fmt.Errorf("uploading .deb: %w", err)
	}

	// Build this source's packages entry
	pkgInfo := PackageInfo{
		Control:  ctrl,
		Filename: filename,
		Size:     int64(len(debData)),
		MD5:      md5sum,
		SHA1:     sha1sum,
		SHA256:   sha256sum,
	}
	packagesEntry := GeneratePackagesFile([]PackageInfo{pkgInfo})

	// Store source's packages entry
	if err := p.s3.Upload(ctx, "meta/"+sourceName+"/packages-entry", packagesEntry, "text/plain"); err != nil {
		return fmt.Errorf("uploading packages entry: %w", err)
	}

	// Lock and regenerate full repo metadata
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.regenerateRepoMetadata(ctx); err != nil {
		return fmt.Errorf("regenerating repo metadata: %w", err)
	}

	// Store new state
	if state != "" {
		if err := p.s3.Upload(ctx, "meta/"+sourceName+"/state", []byte(state), "text/plain"); err != nil {
			return fmt.Errorf("updating state: %w", err)
		}
	}

	slog.Info("Successfully processed", "source", sourceName, "package", ctrl.Package, "version", ctrl.Version)
	return nil
}

func (p *PPA) regenerateRepoMetadata(ctx context.Context) error {
	// List all meta/*/packages-entry files
	keys, err := p.s3.ListPrefix(ctx, "meta/")
	if err != nil {
		return fmt.Errorf("listing meta entries: %w", err)
	}

	// Collect all packages entries
	var allEntries []string
	for _, key := range keys {
		if !strings.HasSuffix(key, "/packages-entry") {
			continue
		}
		data, err := p.s3.Download(ctx, key)
		if err != nil {
			slog.Warn("Failed to download packages entry", "key", key, "error", err)
			continue
		}
		if len(data) > 0 {
			allEntries = append(allEntries, string(data))
		}
	}

	sort.Strings(allEntries)
	packagesData := []byte(strings.Join(allEntries, ""))

	packagesGz, err := GeneratePackagesGz(packagesData)
	if err != nil {
		return fmt.Errorf("compressing Packages: %w", err)
	}

	pkgHash := ComputeFileHash(packagesData)
	pkgHash.Path = "main/binary-amd64/Packages"

	gzHash := ComputeFileHash(packagesGz)
	gzHash.Path = "main/binary-amd64/Packages.gz"

	releaseData := GenerateReleaseFile(p.cfg.Origin, p.cfg.Label, []FileHash{pkgHash, gzHash})

	inRelease, err := p.signer.ClearSign(releaseData)
	if err != nil {
		return fmt.Errorf("clearsigning Release: %w", err)
	}

	releaseGpg, err := p.signer.DetachedSign(releaseData)
	if err != nil {
		return fmt.Errorf("detach-signing Release: %w", err)
	}

	uploads := map[string][]byte{
		"dists/stable/main/binary-amd64/Packages":    packagesData,
		"dists/stable/main/binary-amd64/Packages.gz": packagesGz,
		"dists/stable/Release":                       releaseData,
		"dists/stable/InRelease":                     inRelease,
		"dists/stable/Release.gpg":                   releaseGpg,
		"key.gpg":                                    p.signer.PublicKey(),
	}

	for key, data := range uploads {
		if err := p.s3.Upload(ctx, key, data, ""); err != nil {
			return fmt.Errorf("uploading %s: %w", key, err)
		}
	}

	return nil
}
