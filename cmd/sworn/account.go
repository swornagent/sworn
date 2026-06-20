package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/account"
)

// cmdAccount implements `sworn account`. It loads credentials and displays
// the user's email and tier. If not logged in (no credentials file or expired),
// it prints a message and exits successfully.
func cmdAccount(args []string) int {
	dir := filepath.Dir(account.CredentialsPath())
	creds, err := account.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading credentials: %v\n", err)
		return 1
	}

	if creds == nil || !account.IsLoggedIn(creds) {
		fmt.Println("Not logged in — run `sworn login`")
		return 0
	}

	fmt.Printf("Email: %s\n", creds.Email)
	fmt.Printf("Tier: %s\n", creds.Tier)
	return 0
}