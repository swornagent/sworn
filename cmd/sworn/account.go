package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/swornagent/sworn/internal/account"
)

// cmdAccount implements `sworn account` and `sworn account buy <N>`.
// It loads credentials and displays the user's email, tier, and credit
// balance. If not logged in (no credentials file or expired), it prints
// a message and exits successfully.
//
// `sworn account buy <N>` opens the billing page in the browser to
// purchase N credits (Coach ack pin A — integer credit unit).
func cmdAccount(args []string) int {
	// Subcommand: account buy <N>
	if len(args) > 0 && args[0] == "buy" {
		return cmdAccountBuy(args[1:])
	}

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

	// Display credit balance from cache (Coach ack pin A — integer credits).
	credits, ok := account.LoadCachedCredits()
	if ok {
		fmt.Printf("Credits: %d\n", credits)
	} else {
		fmt.Println("Credits: –")
	}
	fmt.Println("Run `sworn account buy` to add credits")
	return 0
}

// cmdAccountBuy implements `sworn account buy <N>`. It opens the billing
// page in the browser to purchase N credits.
func cmdAccountBuy(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sworn account buy <N>")
		fmt.Fprintln(os.Stderr, "  N = number of credits to purchase")
		return 64
	}

	n, err := strconv.Atoi(args[0])
	if err != nil || n <= 0 {
		fmt.Fprintf(os.Stderr, "Invalid credit amount %q: must be a positive integer\n", args[0])
		return 64
	}

	buyURL := fmt.Sprintf("https://swornagent.com/credits/buy?n=%d", n)
	account.OpenBrowser(buyURL)
	fmt.Fprintf(os.Stderr, "Opening billing page: %s\n", buyURL)
	return 0
}
