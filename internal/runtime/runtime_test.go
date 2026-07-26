package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
)

func runtimePlan(t *testing.T, release, repository, target, marker string) ([]byte, baton.Plan) {
	t.Helper()
	slice := func(id, path string) baton.Slice {
		return baton.Slice{
			ID: id, Outcome: "Deliver " + id + ".",
			Scope:      baton.Scope{Include: []string{path}, Exclude: []string{}},
			Acceptance: []baton.Criterion{{ID: "A-" + id, Text: id + " is exact."}},
			Checks:     []string{"check " + id}, Constraints: []string{"deterministic"},
			DependsOn: []string{}, Consumes: []string{},
		}
	}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef: fmt.Sprintf(
			"github://%s/issues/7#%s",
			repository,
			marker,
		),
		Tracks: []baton.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []baton.Slice{slice("S1", "one.txt")}},
			{ID: "T2", DependsOn: []string{}, Slices: []baton.Slice{slice("S2", "two.txt")}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func encodeSubmission(t *testing.T, submission driver.Submission) string {
	t.Helper()
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(body)
}

func fixtureManifest(t *testing.T) (Manifest, []byte, baton.Plan) {
	t.Helper()
	const (
		runID      = "run-1"
		release    = "release-1"
		repository = "acme/repo"
		target     = "refs/heads/main"
		marker     = "approval-release-1-v1"
	)
	planBytes, plan := runtimePlan(t, release, repository, target, marker)
	submission := func(
		responsibility driver.Responsibility,
	) driver.Submission {
		return driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID(runID, responsibility),
			Responsibility: responsibility,
			Summary:        "Exact " + string(responsibility) + ".",
			Detail:         "Bounded fixture detail.",
		}
	}
	planner := submission(driver.PlannerProposal)
	planner.Plan, _ = driver.NewPlanBytes(planBytes)
	design := submission(driver.ImplementerDesign)
	captain := submission(driver.CaptainReview)
	captain.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission(driver.ImplementerImplementation)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission(driver.WorkVerification)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission(driver.AssemblyVerification)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest := Manifest{
		SchemaVersion: ManifestVersion,
		RunID:         runID, Repository: "/repository", Release: release,
		TargetRef: target, Intent: "Deliver the exact fixture.",
		ActiveTrack: "T1", ActiveSlice: "S1",
		Approval: ApprovalPolicy{
			Repository: repository, Issue: 7, Marker: marker,
			AllowedAuthorIDs:    []int64{42},
			AllowedAssociations: []string{"MEMBER", "OWNER"},
		},
		Driver: FakeDriverConfig{
			Executable: "/bin/true",
			Digest:     "sha256:" + strings.Repeat("a", 64),
			AdapterKey: "fixture", Profile: "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "fixture", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "fixture", Model: "implementer-model"},
			Captain:     driver.RoleSelection{Profile: "fixture", Model: "captain-model"},
			Verifier:    driver.RoleSelection{Profile: "fixture", Model: "verifier-model"},
		},
		Limits: driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
		Submissions: ScriptedSubmissions{
			PlannerProposal:           encodeSubmission(t, planner),
			ImplementerDesign:         encodeSubmission(t, design),
			CaptainReview:             encodeSubmission(t, captain),
			ImplementerImplementation: encodeSubmission(t, implementation),
			WorkVerification:          encodeSubmission(t, work),
			AssemblyVerification:      encodeSubmission(t, assembly),
		},
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, body, plan
}

func TestManifestIsClosedCanonicalAndBindsEverySubmission(t *testing.T) {
	t.Parallel()

	manifest, body, _ := fixtureManifest(t)
	admitted, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.value.RunID != manifest.RunID ||
		admitted.digest != sha256Digest(body) {
		t.Fatalf("admission = %#v", admitted)
	}
	unknown := append([]byte(nil), body...)
	unknown = []byte(strings.Replace(
		string(unknown),
		`"schema_version":"sworn.runtime-manifest/v1"`,
		`"schema_version":"sworn.runtime-manifest/v1","unknown":true`,
		1,
	))
	if _, err := admitManifest(unknown); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("unknown manifest field = %v", err)
	}
	duplicate := []byte(strings.Replace(
		string(body),
		`"run_id":"run-1"`,
		`"run_id":"run-1","run_id":"run-1"`,
		1,
	))
	if _, err := admitManifest(duplicate); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("duplicate manifest field = %v", err)
	}
	pretty := append([]byte("{\n"), body[1:]...)
	if _, err := admitManifest(pretty); !IsCode(err, "NONCANONICAL_MANIFEST") {
		t.Fatalf("noncanonical manifest = %v", err)
	}
	mutated := manifest
	mutated.Submissions.CaptainReview = mutated.Submissions.WorkVerification
	mutatedBody, err := json.Marshal(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(append(mutatedBody, '\n')); !IsCode(err, "INVALID_SCRIPTED_SUBMISSION") {
		t.Fatalf("responsibility substitution = %v", err)
	}
}

func TestGitHubApprovalIsUniqueUneditedGETOnlyAndDigestBound(t *testing.T) {
	t.Parallel()

	manifestValue, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	created := "2026-07-26T01:02:03Z"
	commentBody := fmt.Sprintf(
		"baton-plan-approval/v1\nmarker: %s\ndecision: approved\nrepository: %s\nissue: %d\nplan_digest: %s\n",
		manifestValue.Approval.Marker,
		manifestValue.Approval.Repository,
		manifestValue.Approval.Issue,
		plan.Digest(),
	)
	var (
		mu      sync.Mutex
		methods []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		methods = append(methods, request.Method)
		mu.Unlock()
		if request.URL.Path != "/repos/acme/repo/issues/7/comments" ||
			request.URL.Query().Get("per_page") != "100" ||
			request.URL.Query().Get("page") != "1" {
			http.Error(writer, "unexpected request", http.StatusBadRequest)
			return
		}
		writer.Header().Set("ETag", `"fixture-etag"`)
		_ = json.NewEncoder(writer).Encode([]githubComment{{
			ID: 99, HTMLURL: "https://github.com/acme/repo/issues/7#issuecomment-99",
			Body: commentBody, AuthorAssociation: "MEMBER",
			CreatedAt: created, UpdatedAt: created,
			User: struct {
				ID    int64  `json:"id"`
				Login string `json:"login"`
			}{ID: 42, Login: "approver"},
		}})
	}))
	defer server.Close()
	resolver := newFixtureApprovalResolver(server.URL, server.Client())
	admission, err := resolver.resolve(context.Background(), manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	if admission.planDigest != plan.Digest() ||
		admission.reference != plan.Metadata().ApprovalRef ||
		admission.commentID != 99 ||
		!strings.Contains(string(admission.evidence), `"match_count":1`) ||
		!strings.Contains(string(admission.evidence), `"raw_body_digest":"`+sha256Digest([]byte(commentBody))+`"`) {
		t.Fatalf("approval admission = %#v, evidence = %s", admission, admission.evidence)
	}
	serialized, err := json.Marshal(admission)
	if err != nil || string(serialized) != "{}" {
		t.Fatalf("approval admission serialized as %s, err = %v", serialized, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Fatalf("approval methods = %v", methods)
	}
}

func TestGitHubApprovalRejectsEditedDuplicateAndRedirectedMarkers(t *testing.T) {
	t.Parallel()

	manifestValue, manifestBody, plan := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(
		"baton-plan-approval/v1\nmarker: %s\ndecision: approved\nrepository: %s\nissue: %d\nplan_digest: %s\n",
		manifestValue.Approval.Marker,
		manifestValue.Approval.Repository,
		manifestValue.Approval.Issue,
		plan.Digest(),
	)
	valid := githubComment{
		ID: 99, HTMLURL: "https://github.com/acme/repo/issues/7#issuecomment-99",
		Body: body, AuthorAssociation: "MEMBER",
		CreatedAt: "2026-07-26T01:02:03Z", UpdatedAt: "2026-07-26T01:02:03Z",
		User: struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		}{ID: 42, Login: "approver"},
	}
	for name, response := range map[string][]githubComment{
		"edited": func() []githubComment {
			edited := valid
			edited.UpdatedAt = "2026-07-26T01:03:03Z"
			return []githubComment{edited}
		}(),
		"duplicate": {valid, valid},
		"wrong author": func() []githubComment {
			wrong := valid
			wrong.User.ID = 43
			return []githubComment{wrong}
		}(),
		"noncanonical url": func() []githubComment {
			noncanonical := valid
			noncanonical.HTMLURL += "?redirect=1"
			return []githubComment{noncanonical}
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(response)
			}))
			defer server.Close()
			_, err := newFixtureApprovalResolver(server.URL, server.Client()).
				resolve(context.Background(), manifest, plan)
			if err == nil {
				t.Fatal("unsafe approval was admitted")
			}
		})
	}
	encoded, err := json.Marshal([]githubComment{valid})
	if err != nil {
		t.Fatal(err)
	}
	duplicateKeyBody := bytes.Replace(
		encoded,
		[]byte(`"id":99`),
		[]byte(`"id":99,"id":99`),
		1,
	)
	if bytes.Equal(duplicateKeyBody, encoded) {
		t.Fatal("fixture did not insert a duplicate JSON key")
	}
	duplicateKeyServer := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(duplicateKeyBody)
		},
	))
	defer duplicateKeyServer.Close()
	if _, err := newFixtureApprovalResolver(
		duplicateKeyServer.URL,
		duplicateKeyServer.Client(),
	).resolve(context.Background(), manifest, plan); !IsCode(err, "APPROVAL_UNAVAILABLE") {
		t.Fatalf("duplicate JSON key = %v", err)
	}
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode([]githubComment{valid})
	}))
	defer redirectTarget.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, redirectTarget.URL, http.StatusFound)
	}))
	defer redirect.Close()
	if _, err := newFixtureApprovalResolver(redirect.URL, redirect.Client()).
		resolve(context.Background(), manifest, plan); !IsCode(err, "APPROVAL_UNAVAILABLE") {
		t.Fatalf("redirect = %v", err)
	}
}

func TestInvocationIdentityIsStableAcrossResume(t *testing.T) {
	t.Parallel()

	for _, responsibility := range []driver.Responsibility{
		driver.PlannerProposal,
		driver.ImplementerDesign,
		driver.CaptainReview,
		driver.ImplementerImplementation,
		driver.WorkVerification,
		driver.AssemblyVerification,
	} {
		got := invocationID("run-1", responsibility)
		want := "run-1/" + string(responsibility) + "/1"
		if got != want {
			t.Fatalf("invocation ID = %q, want %q", got, want)
		}
	}
	if _, err := time.Parse(time.RFC3339, "2026-07-26T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
}
