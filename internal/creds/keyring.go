// Package creds stores provider API keys in the OS keyring (Windows Credential
// Manager / macOS Keychain / Linux Secret Service) behind core.CredentialStore.
// An environment variable overrides the keyring so headless/CI setups work
// without a backend. Keys are never written to config files or logs.
package creds

import (
	"errors"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// keyringService is the fixed namespace under the OS keyring. Changing it would
// orphan every key already saved, so it stays constant.
const keyringService = "baton"

// Keyring is the default CredentialStore.
type Keyring struct{}

// New returns a keyring-backed credential store.
func New() *Keyring { return &Keyring{} }

// Get returns the key for a service. An environment variable named
// <SERVICE>_API_KEY (e.g. OPENCODE_API_KEY) wins, so CI and Docker keep working;
// otherwise the OS keyring is consulted. A missing key yields ("", nil).
func (Keyring) Get(service string) (string, error) {
	if v := os.Getenv(strings.ToUpper(service) + "_API_KEY"); v != "" {
		return v, nil
	}
	v, err := keyring.Get(keyringService, service)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return v, err
}

// Set stores a key. An empty value is rejected — use Delete to remove.
func (Keyring) Set(service, value string) error {
	if value == "" {
		return errors.New("refusing to store an empty key; use delete instead")
	}
	return keyring.Set(keyringService, service, value)
}

// Delete removes a key. Deleting a missing key is not an error (idempotent).
func (Keyring) Delete(service string) error {
	err := keyring.Delete(keyringService, service)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
