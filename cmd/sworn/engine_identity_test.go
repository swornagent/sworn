package main

import (
	"strings"
	"testing"
)

// Every machine identity the engine commits with must live on a domain the
// project controls. The original plan@sworn.dev / records@sworn.dev were on
// a domain that is not ours: a records commit's author email is an identity
// other systems act on (Vercel refused fired's release-wt deploys as a
// non-member author, 2026-09-02), and whoever holds the domain inherits the
// attribution.
func TestEngineIdentitiesLiveOnAProjectOwnedDomain(t *testing.T) {
	if engineIdentityDomain != "sworn.sh" {
		t.Fatalf("engine identity domain = %q, want the project-owned sworn.sh", engineIdentityDomain)
	}
	for _, identity := range []struct {
		label string
		email string
	}{
		{"plan", planEngineIdentity.Email},
		{"migration", migrationEngineIdentity.Email},
	} {
		if !strings.HasSuffix(identity.email, "@"+engineIdentityDomain) {
			t.Fatalf("%s engine identity %q is not on %s", identity.label, identity.email, engineIdentityDomain)
		}
		if strings.Contains(identity.email, "sworn.dev") {
			t.Fatalf("%s engine identity %q still names the foreign domain", identity.label, identity.email)
		}
	}
}
