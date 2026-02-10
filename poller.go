package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Poller struct {
	cfg    *Config
	s3     *S3Client
	signer *GPGSigner
}

func NewPoller(cfg *Config, s3 *S3Client, signer *GPGSigner) *Poller {
	return &Poller{cfg: cfg, s3: s3, signer: signer}
}

func (p *Poller) Run(ctx context.Context) {
	p.poll(ctx)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	log.Println("Polling for new Discord .deb...")

	resp, err := p.httpWithRetry(ctx, p.cfg.DownloadURL, "HEAD")
	if err != nil {
		log.Printf("HEAD request failed: %v", err)
		return
	}
	resp.Body.Close()

	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = resp.Header.Get("Content-Length")
	}

	lastEtag, err := p.s3.Download(ctx, "meta/last-etag")
	if err == nil && string(lastEtag) == etag && etag != "" {
		log.Println("No new version detected")
		return
	}

	log.Println("New version detected, downloading...")
	if err := p.processNewVersion(ctx, etag); err != nil {
		log.Printf("Error processing new version: %v", err)
	}
}

func (p *Poller) httpWithRetry(ctx context.Context, url, method string) (*http.Response, error) {
	for attempt := range 3 {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		resp.Body.Close()

		wait := 30 * time.Second * time.Duration(1<<attempt)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		log.Printf("Rate limited (429), retry-after %v (attempt %d/3)", wait, attempt+1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("rate limited after 3 retries")
}

func (p *Poller) processNewVersion(ctx context.Context, etag string) error {
	resp, err := p.httpWithRetry(ctx, p.cfg.DownloadURL, "GET")
	if err != nil {
		return fmt.Errorf("downloading .deb: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d downloading .deb", resp.StatusCode)
	}

	debData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading .deb: %w", err)
	}

	ctrl, err := ParseDebControl(bytes.NewReader(debData))
	if err != nil {
		return fmt.Errorf("parsing .deb: %w", err)
	}

	filename := fmt.Sprintf("pool/d/discord/%s-%s.deb", ctrl.Package, ctrl.Version)

	md5sum := fmt.Sprintf("%x", md5.Sum(debData))
	sha1sum := fmt.Sprintf("%x", sha1.Sum(debData))
	sha256sum := fmt.Sprintf("%x", sha256.Sum256(debData))

	log.Printf("Uploading %s (%d bytes)", filename, len(debData))
	if err := p.s3.Upload(ctx, filename, debData, "application/vnd.debian.binary-package"); err != nil {
		return fmt.Errorf("uploading .deb: %w", err)
	}

	pkgInfo := PackageInfo{
		Control:  ctrl,
		Filename: filename,
		Size:     int64(len(debData)),
		MD5:      md5sum,
		SHA1:     sha1sum,
		SHA256:   sha256sum,
	}
	packages := []PackageInfo{pkgInfo}

	packagesData := GeneratePackagesFile(packages)
	packagesGz, err := GeneratePackagesGz(packagesData)
	if err != nil {
		return fmt.Errorf("compressing Packages: %w", err)
	}

	pkgHash := ComputeFileHash(packagesData)
	pkgHash.Path = "main/binary-amd64/Packages"

	gzHash := ComputeFileHash(packagesGz)
	gzHash.Path = "main/binary-amd64/Packages.gz"

	releaseData := GenerateReleaseFile([]FileHash{pkgHash, gzHash})

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

	if etag != "" {
		if err := p.s3.Upload(ctx, "meta/last-etag", []byte(etag), "text/plain"); err != nil {
			return fmt.Errorf("updating last-etag: %w", err)
		}
	}

	log.Printf("Successfully processed %s version %s", ctrl.Package, ctrl.Version)
	return nil
}
