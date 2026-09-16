package main

import (
	"os"

	"github.com/swornagent/sworn/internal/driver"
)

func main() {
	profile := driver.FakeProfile(os.Getenv("PROTOCOL_FAKE_PROFILE"))
	if profile == "" {
		profile = driver.FakeCompleted
	}
	os.Exit(driver.RunFakeCommand(
		os.Args[1:],
		os.Stdin,
		os.Stdout,
		os.Stderr,
		profile,
	))
}
