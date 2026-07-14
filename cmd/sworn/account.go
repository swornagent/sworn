package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/style"
)

func init() {
	// T3-commercial owns the account verb (S07-paging adds set-webhook + notifications).
	// Self-registration via init() — never edit cmd/sworn/main.go to add a command.
	command.Register(command.Command{
		Name:    "account",
		Summary: "show account status, buy credits, and configure webhook notifications",
		Run:     cmdAccount,
	})
}

// cmdAccount implements `sworn account` and its subcommands:
//
//	sworn account                — display email, tier, credits
//	sworn account buy <N>        — open billing page for N credits
//	sworn account set-webhook <url> — store webhook URL
//	sworn account notifications  — show webhook URL + email status
func cmdAccount(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "buy":
			return cmdAccountBuy(args[1:])
		case "set-webhook":
			return cmdAccountSetWebhook(args[1:])
		case "notifications":
			return cmdAccountNotifications(args[1:])
		}
	}

	creds, err := account.LoadDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading credentials: %v\n", err)
		return 1
	}

	if creds == nil || !account.IsLoggedIn(creds) {
		fmt.Println(style.Dim("Not logged in — run `sworn login`"))
		return 0
	}

	fmt.Printf("Email: %s\n", style.Accent(creds.Email))
	fmt.Printf("Tier: %s\n", style.Accent(creds.Tier))

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

// cmdAccountSetWebhook implements `sworn account set-webhook <url>`.
// It stores the webhook URL in the credentials file. If the credentials
// file does not exist (not logged in), it creates a minimal file with
// just the webhook URL.
func cmdAccountSetWebhook(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sworn account set-webhook <url>")
		fmt.Fprintln(os.Stderr, "  url = webhook endpoint to POST notifications to")
		return 64
	}

	webhookURL := strings.TrimSpace(args[0])
	if webhookURL == "" {
		fmt.Fprintln(os.Stderr, "Error: webhook URL must not be empty")
		return 64
	}

	creds, err := account.LoadDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading credentials: %v\n", err)
		return 1
	}

	if creds == nil {
		creds = &account.Credentials{}
	}
	creds.WebhookURL = webhookURL

	if err := account.SaveDefault(*creds); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving webhook URL: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Webhook URL set to: %s\n", webhookURL)
	return 0
}

// cmdAccountNotifications implements `sworn account notifications`.
// It prints the current webhook URL and whether email notifications
// are enabled (account is logged in).
func cmdAccountNotifications(args []string) int {
	creds, err := account.LoadDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading credentials: %v\n", err)
		return 1
	}

	if creds == nil {
		fmt.Println(style.Dim("Not logged in — run `sworn login`"))
		fmt.Println("No webhook configured — run `sworn account set-webhook <url>`")
		return 0
	}

	if creds.WebhookURL != "" {
		fmt.Printf("Webhook URL: %s\n", creds.WebhookURL)
	} else {
		fmt.Println("No webhook configured — run `sworn account set-webhook <url>`")
	}

	if account.IsLoggedIn(creds) {
		fmt.Printf("Email notifications: enabled (sent to %s)\n", creds.Email)
	} else {
		fmt.Println("Email notifications: disabled (not logged in)")
	}

	return 0
}
