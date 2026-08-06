package skill

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func writeSkill(t *testing.T, root, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(root, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func generatedLegacyBody(name, release, digest string) []byte {
	return []byte("---\nname: " + name + "\ndescription: \"legacy\"\n---\n\n" +
		"<!-- baton-skill\nrelease: " + release + "\n" +
		"generator-version: baton.skill-generator/v1\n" +
		"operation-version: baton.operation/v2\n" +
		"operation-sha256: " + digest + "\n-->\n\n" +
		"Legacy standalone role prose.\n")
}

// homeWithRoots creates the given supported roots' parent directories (as a
// real agent-tool install would) so Install treats them as supported.
func homeWithRoots(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, root := range SupportedRoots(home) {
		if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestInstallCleanHomeInstallsOneSwornSkill(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)

	report, err := Install(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MigratedStubs) != 0 {
		t.Fatalf("clean install migrated stubs: %v", report.MigratedStubs)
	}
	if len(report.InstalledPaths) != 1 {
		t.Fatalf("clean install paths = %v, want exactly one", report.InstalledPaths)
	}
	body, err := os.ReadFile(report.InstalledPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte("---\nname: sworn\n")) ||
		!bytes.Contains(body, []byte("<!-- sworn-skill\nversion: "+baton.RoleAssetsVersion)) {
		t.Fatalf("installed sworn skill does not name itself and its identity: %s", body)
	}
}

func TestInstallUpgradesAllRecognizedLegacySkills(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)
	root := SupportedRoots(home)[0]
	var legacyPaths []string
	for _, name := range LegacyNames {
		legacyPaths = append(legacyPaths, writeSkill(t, root, name, generatedLegacyBody(name, "v1.0.0-rc.14", "sha256:aaaa")))
	}

	report, err := Install(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MigratedStubs) != len(LegacyNames) {
		t.Fatalf("migrated stubs = %v, want %d entries", report.MigratedStubs, len(LegacyNames))
	}
	for _, path := range legacyPaths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("operation:")) {
			t.Fatalf("legacy skill %s still contains actionable operation prose", path)
		}
	}
	if len(report.InstalledPaths) != 1 || filepath.Dir(filepath.Dir(report.InstalledPaths[0])) != root {
		t.Fatalf("installed sworn skill paths = %v, want one skill under %s", report.InstalledPaths, root)
	}
}

func TestInstallHandlesMultipleRootPrecedence(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)
	roots := SupportedRoots(home)
	writeSkill(t, roots[0], "baton-implement", generatedLegacyBody("baton-implement", "v1.0.0-rc.13", "sha256:aaaa"))
	writeSkill(t, roots[1], "baton-implement", generatedLegacyBody("baton-implement", "v1.0.0-rc.14", "sha256:bbbb"))

	report, err := Install(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MigratedStubs) != 2 {
		t.Fatalf("migrated stubs = %v, want both roots migrated", report.MigratedStubs)
	}
	if len(report.InstalledPaths) != 2 {
		t.Fatalf("installed paths = %v, want the sworn skill present at both roots", report.InstalledPaths)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)
	root := SupportedRoots(home)[0]
	writeSkill(t, root, "baton-verify", generatedLegacyBody("baton-verify", "v1.0.0-rc.14", "sha256:cccc"))

	first, err := Install(home)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Install(home)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	if len(second.MigratedStubs) != len(first.MigratedStubs) ||
		len(second.InstalledPaths) != len(first.InstalledPaths) {
		t.Fatalf("install is not idempotent: first = %#v, second = %#v", first, second)
	}
}

func TestInstallHandlesPartialStaleState(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)
	root := SupportedRoots(home)[0]
	writeSkill(t, root, "baton-plan", generatedLegacyBody("baton-plan", "v1.0.0-rc.14", "sha256:dddd"))

	report, err := Install(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MigratedStubs) != 1 {
		t.Fatalf("migrated stubs = %v, want exactly the one present legacy skill", report.MigratedStubs)
	}
	for _, name := range LegacyNames {
		if name == "baton-plan" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("absent legacy skill %s was created", name)
		}
	}
}

func TestInstallRejectsModifiedCollisionWithoutMutation(t *testing.T) {
	t.Parallel()
	home := homeWithRoots(t)
	root := SupportedRoots(home)[0]
	modified := writeSkill(t, root, "baton-merge", []byte("---\nname: baton-merge\n---\n\nhand-edited local notes\n"))
	before, err := os.ReadFile(modified)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Install(home)
	var collision *CollisionError
	if err == nil {
		t.Fatal("Install accepted a modified legacy skill")
	}
	if !isCollision(err, &collision) || collision.Path != modified {
		t.Fatalf("Install error = %v, want a CollisionError naming %s", err, modified)
	}
	after, readErr := os.ReadFile(modified)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(before) != string(after) {
		t.Fatal("Install mutated a modified collision instead of failing closed")
	}
	if _, statErr := os.Stat(filepath.Join(root, Name, "SKILL.md")); !os.IsNotExist(statErr) {
		t.Fatal("Install installed the sworn skill despite an unresolved collision")
	}
}

func TestInstalledSkillNamesTruthfulRoleAssetsIdentity(t *testing.T) {
	t.Parallel()
	if !strings.Contains(string(swornSkillContent()), baton.RoleAssetsVersion) {
		t.Fatal("sworn skill content does not name Sworn's own role-assets identity")
	}
}

func isCollision(err error, target **CollisionError) bool {
	value, ok := err.(*CollisionError)
	if ok {
		*target = value
	}
	return ok
}
