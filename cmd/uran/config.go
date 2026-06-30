package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// credentials is the persisted CLI session: which API and the bearer token.
type credentials struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
}

// credentialsPath returns ~/<config>/uran/credentials.json.
func credentialsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "uran", "credentials.json"), nil
}

// loadCredentials reads the saved session, or returns a helpful error telling
// the user to log in.
func loadCredentials() (credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return credentials{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return credentials{}, fmt.Errorf("not logged in (run `uran login`): %w", err)
	}
	var c credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return credentials{}, fmt.Errorf("corrupt credentials at %s: %w", path, err)
	}
	return c, nil
}

// saveCredentials writes the session with owner-only permissions.
func saveCredentials(c credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
