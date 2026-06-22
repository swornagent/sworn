package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/command"
)

func init() {
	// T3-commercial owns the login/logout verbs (S06a-sworn-login-auth).
	// Self-registration via init() — never edit cmd/sworn/main.go to add a command.
	command.Register(command.Command{
		Name:    "login",
		Summary: "authenticate with SwornAgent via device-code OAuth2 flow",
		Run:     cmdLogin,
	})
	command.Register(command.Command{
		Name:    "logout",
		Summary: "remove local SwornAgent credentials",
		Run:     cmdLogout,
	})
}

// authURL is the production SwornAgent auth endpoint. It can be overridden at
// build time via -ldflags "-X main.authURL=https://custom.auth.example.com".
// At runtime, SWORN_AUTH_URL env var takes precedence.
//
// Coach decision (approved-ack.md pin 4): SWORN_AUTH_URL env var with ldflags
// compile-time fallback.
var authURL = "https://auth.sworn.sh"

// resolveAuthEndpoint returns the auth endpoint URL with precedence:
// 1. SWORN_AUTH_URL env var
// 2. compile-time authURL (ldflags)
func resolveAuthEndpoint() string {
	if env := os.Getenv("SWORN_AUTH_URL"); env != "" {
		return env
	}
	return authURL
}

// cmdLogin implements `sworn login`. It performs a device-code OAuth2 flow,
// saves the resulting credentials, and prints a success message.
func cmdLogin(args []string) int {
	endpoint := resolveAuthEndpoint()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Authenticating with SwornAgent...\n")

	token, email, err := account.DeviceCodeFlow(ctx, endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		return 1
	}

	creds := account.Credentials{
		Token:     token,
		Email:     email,
		Tier:      "free", // default tier; server may override on auth
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	dir := filepath.Dir(account.CredentialsPath())
	if err := account.Save(creds, dir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Logged in as %s\n", email)
	return 0
}

// cmdLogout implements `sworn logout`. It removes the credentials file and
// prints a message. If no credentials file exists, it is a silent no-op
// (Coach pin 2 / Captain pin 1: suppress os.ErrNotExist).
func cmdLogout(args []string) int {
	path := account.CredentialsPath()
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No credentials file — no-op, print success anyway.
			fmt.Fprintln(os.Stderr, "Logged out")
			return 0
		}
		fmt.Fprintf(os.Stderr, "Logout failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "Logged out")
	return 0
}
