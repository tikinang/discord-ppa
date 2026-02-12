package ppa

import "context"

// Source represents a package source that can be polled for new versions.
type Source interface {
	// Name returns the source identifier (used for state storage keys, logging).
	Name() string

	// Description returns a human-readable description of how the package
	// is fetched and built, displayed on the index page.
	Description() string

	// Check returns a state string representing the current upstream version.
	// The PPA compares this with the previously stored state to detect changes.
	Check(ctx context.Context) (state string, err error)

	// Fetch downloads or builds the .deb package bytes.
	// Called only when Check returns a different state than stored.
	Fetch(ctx context.Context) (deb []byte, err error)
}
