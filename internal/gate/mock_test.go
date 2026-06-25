package gate

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- mock pattern detection tests ---

func TestMockPatternRe(t *testing.T) {
	tests := []struct {
		line  string
		match bool
		kind  string
		val   string
	}{
		// Go mock patterns.
		{`srv := httptest.NewServer(handler)`, true, "httptest", "httptest."},
		{`ctrl := gomock.NewController(t)`, true, "gomock", "gomock."},
		{`import "github.com/stretchr/testify/mock"`, true, "testify/mock", "testify/mock"},
		{`m := NewMockClient(ctrl)`, true, "NewMock", "NewMockClient"},
		{`mockDB := MockDB{}`, true, "Mock", "MockDB{"},
		{`svc.MockCall("ping")`, true, ".Mock", ".MockCall("},		{`fake := fake.NewClient()`, true, "fake.", "fake."},
		{`stub := stub.NewUserRepo()`, true, "stub.New", "stub.New"},
		{`mockSvc := mock.New(ctrl)`, true, "mock.New", "mock.New"},

		// TypeScript mock patterns.
		{`vi.fn().mockReturnValue(true)`, true, "vitest-mock", "vi.fn"},
		{`jest.fn().mockImplementation(() => true)`, true, "jest-mock", "jest.fn"},
		{`const spy = vi.spyOn(obj, 'method')`, true, "vitest-mock", "vi.spyOn"},
		{`sinon.stub(obj, 'method')`, true, "sinon", "sinon."},
		{`const scope = nock('https://api.example.com')`, true, "nock", "nock("},
		{`import { http, HttpResponse } from 'msw'`, true, "msw", "msw"},

		// Python mock patterns.
		{`from unittest.mock import MagicMock`, true, "unittest.mock", "unittest.mock"},
		{`m = MagicMock()`, true, "MagicMock", "MagicMock"},
		{`@patch('module.func')`, true, "patch", "patch("},
		{`monkeypatch.setattr('os.path', fake)`, true, "pytest-monkeypatch", "monkeypatch"},
		{`import responses`, true, "responses-lib", "responses"},

		// Testdata / fixture directories.
		{`data := testdata/input.json`, true, "testdata/", "testdata/"},
		{`fixtures/user.json`, true, "fixtures/", "fixtures/"},

		// Negative cases.
		{`const result = calculate(1, 2)`, false, "", ""},
		{`func TestRealIntegration(t *testing.T) {`, false, "", ""},
		{`assert.Equal(t, expected, actual)`, false, "", ""},
	}

	for _, tt := range tests {
		matched := false
		var matchKind, matchVal string
		for _, pat := range mockPatterns {
			if m := pat.re.FindString(tt.line); m != "" {
				matched = true
				matchKind = pat.kind
				matchVal = m
				break
			}
		}
		if matched != tt.match {
			t.Errorf("mockPatterns: line=%q → match=%v, want %v", tt.line, matched, tt.match)
		} else if matched {
			if matchKind != tt.kind {
				t.Errorf("mockPatterns: line=%q → kind=%q, want %q", tt.line, matchKind, tt.kind)
			}
			if matchVal != tt.val {
				t.Errorf("mockPatterns: line=%q → val=%q, want %q", tt.line, matchVal, tt.val)
			}
		}
	}
}

// --- real-infra pattern detection tests ---

func TestRealInfraPatternRe(t *testing.T) {
	tests := []struct {
		line  string
		match bool
		kind  string
	}{
		// Localhost / loopback.
		{`conn, _ := net.Dial("tcp", "localhost:5432")`, true, "localhost"},
		{`url := "http://127.0.0.1:3000/api"`, true, "127.0.0.1"},
		{`http.ListenAndServe("0.0.0.0:8080", nil)`, true, "0.0.0.0"},

		// Database env vars.
		{`DATABASE_URL=postgres://...`, true, "DATABASE_URL"},
		{`os.Getenv("DB_URL")`, true, "DB_URL"},
		{`POSTGRES_HOST=localhost`, true, "POSTGRES"},
		{`MYSQL_USER=root`, true, "MYSQL"},
		{`REDIS_URL=redis://localhost:6379`, true, "REDIS"},
		{`MONGODB_URI=mongodb://localhost:27017`, true, "MONGODB"},

		// Connection strings.
		{`db, _ := sql.Open("postgres", "postgres://user:pass@localhost/db")`, true, "postgres-uri"},
		{`mysql://root@localhost/test`, true, "mysql-uri"},
		{`mongodb://localhost:27017`, true, "mongodb-uri"},
		{`redis://:password@localhost:6379`, true, "redis-uri"},

		// Auth & secrets.
		{`AUTH0_DOMAIN=dev-xxx.auth0.com`, true, "AUTH0_DOMAIN"},
		{`AUTH0_CLIENT_ID=abc123`, true, "AUTH0_CLIENT_ID"},
		{`AUTH0_CLIENT_SECRET=secret`, true, "AUTH0_CLIENT_SECRET"},
		{`STRIPE_KEY=sk_test_...`, true, "STRIPE_KEY"},
		{`STRIPE_SECRET=whsec_...`, true, "STRIPE_SECRET"},
		{`API_KEY=my-api-key`, true, "API_KEY"},
		{`SECRET_KEY=super-secret`, true, "SECRET_KEY"},

		// env vars in code.
		{`process.env.DATABASE_URL`, true, "DATABASE_URL"},
		{`os.Getenv("AUTH0_DOMAIN")`, true, "AUTH0_DOMAIN"},
		{`load_dotenv()`, true, "dotenv"},

		// HTTP calls.
		{`resp, err := http.Get("https://api.example.com")`, true, "http.Get"},
		{`resp, err := http.Post(url, "application/json", body)`, true, "http.Post"},
		{`const res = await fetch('/api/data')`, true, "fetch"},
		{`axios.get('/api/users')`, true, "axios"},
		{`requests.get("https://api.example.com")`, true, "requests"},
		{`urllib.request.urlopen(...)`, true, "urllib.request"},

		// Cloud SDKs.
		{`aws.s3.listBuckets()`, true, "aws-sdk"},
		{`gcp.storage.bucket('my-bucket')`, true, "gcp-sdk"},
		{`azure.blob.createContainer('container')`, true, "azure-sdk"},

		// Filesystem.
		{`f, _ := os.Open("/etc/config")`, true, "os.Open"},
		{`f, _ := os.Create("output.txt")`, true, "os.Create"},
		{`data, _ := os.ReadFile("/etc/hosts")`, true, "os.ReadFile"},

		// Negative cases.
		{`const host = "example.com"`, false, ""},
		{`mockServer := NewMockServer()`, false, ""},
		{`t.Setenv("TEST_VAR", "value")`, false, ""},
	}

	for _, tt := range tests {
		matched := false
		var matchKind string
		for _, pat := range realInfraPatterns {
			if m := pat.re.FindString(tt.line); m != "" {
				matched = true
				matchKind = pat.kind
				break
			}
		}
		if matched != tt.match {
			t.Errorf("realInfraPatterns: line=%q → match=%v, want %v", tt.line, matched, tt.match)
		} else if matched && matchKind != tt.kind {
			t.Errorf("realInfraPatterns: line=%q → kind=%q, want %q", tt.line, matchKind, tt.kind)
		}
	}
}

// --- findMocksInFile tests ---

func TestFindMocksInFile(t *testing.T) {
	lines := []string{
		"package test",
		"",
		`import "net/http/httptest"`,
		"",
		"func TestWithMock(t *testing.T) {",
		`	srv := httptest.NewServer(handler)`,
		`	ctrl := gomock.NewController(t)`,
		`	mockDB := NewMockDB(ctrl)`,
		`	defer srv.Close()`,
		"}",
	}

	usages, lineNos := findMocksInFile("test.go", lines, nil)
	if len(usages) == 0 {
		t.Fatal("expected mock usages, got none")
	}

	// Check each usage.
	expected := map[int]string{
		6: "httptest",
		7: "gomock",
		8: "NewMock",
	}
	for _, u := range usages {
		if exp, ok := expected[u.Line]; ok {
			if u.Kind != exp {
				t.Errorf("line %d: kind=%q, want %q", u.Line, u.Kind, exp)
			}
			delete(expected, u.Line)
		}
	}
	if len(expected) > 0 {
		t.Errorf("missing usages at lines: %v", expected)
	}

	if len(lineNos) != 3 {
		t.Errorf("lineNos len=%d, want 3", len(lineNos))
	}
}

// --- findInfraInFile tests ---

func TestFindInfraInFile(t *testing.T) {
	lines := []string{
		"package test",
		"",
		"func TestRealInfra(t *testing.T) {",
		`	os.Getenv("DATABASE_URL")`,
		`	http.Get("http://localhost:5432")`,
		"}",
	}

	refs, lineNos := findInfraInFile("test.go", lines, nil)
	if len(refs) < 2 {
		t.Fatalf("expected ≥2 infra refs, got %d: %v", len(refs), refs)
	}

	// Check for the infra patterns we expect.
	foundDBURL := false
	foundHTTP := false
	for _, r := range refs {
		if r.Kind == "DATABASE_URL" {
			foundDBURL = true
		}
		if r.Kind == "http.Get" {
			foundHTTP = true
		}
	}
	if !foundDBURL {
		t.Errorf("expected to find DATABASE_URL ref, got: %v", refs)
	}
	if !foundHTTP {
		t.Errorf("expected to find http.Get ref, got: %v", refs)
	}
	if len(lineNos) < 2 {
		t.Errorf("lineNos len=%d, want ≥2", len(lineNos))
	}
}

// --- hasMockBoundaryInFile tests ---

func TestHasMockBoundaryInFile(t *testing.T) {
	tests := []struct {
		lines    []string
		hasBound bool
	}{
		{[]string{"// @mock-boundary: accepted for integration tests"}, true},
		{[]string{"/* @mock-boundary */"}, true},
		{[]string{"const boundary = true"}, false},
		{[]string{"// this is a mock but we declare it below"}, false},
		{[]string{"// @mock-boundary"}, true},
	}

	for _, tt := range tests {
		got := hasMockBoundaryInFile(tt.lines)
		if got != tt.hasBound {
			t.Errorf("hasMockBoundaryInFile(%v) = %v, want %v", tt.lines, got, tt.hasBound)
		}
	}
}

// --- MockDeferrals.HasMockBoundary tests ---

func TestMockDeferralsHasMockBoundary(t *testing.T) {
	tests := []struct {
		name string
		d    *MockDeferrals
		want bool
	}{
		{
			name: "mock in what field",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{What: "use httptest mocks"}}},
			want: true,
		},
		{
			name: "boundary in why field",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{Why: "no mock boundary declared"}}},
			want: true,
		},
		{
			name: "stub in what",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{What: "stub out the DB"}}},
			want: true,
		},
		{
			name: "fixture in why",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{Why: "uses test fixtures"}}},
			want: true,
		},
		{
			name: "seed in what",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{What: "seed the database"}}},
			want: true,
		},
		{
			name: "no mock mention",
			d:    &MockDeferrals{Entries: []MockDeferralEntry{{What: "deferred colours", Why: "no tokens yet"}}},
			want: false,
		},
		{
			name: "empty deferrals",
			d:    &MockDeferrals{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.HasMockBoundary()
			if got != tt.want {
				t.Errorf("HasMockBoundary() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- isMockExempt tests ---

func TestIsMockExempt(t *testing.T) {
	overrides := &MockOverrides{
		MockOverrides: []MockOverrideRule{
			{RuleID: "mock-boundary", File: "testdata/integration_test.go"},
			{RuleID: "mock-boundary", File: ""},
			{RuleID: "no-mock-boundary", File: "e2e/login.spec.ts"},
		},
	}

	tests := []struct {
		file   string
		exempt bool
	}{
		{"testdata/integration_test.go", true},
		{"e2e/login.spec.ts", true},
		{"e2e/other.spec.ts", true}, // blanket exempt (File: "")
		{"internal/gate/mock.go", true}, // blanket exempt
	}

	for _, tt := range tests {
		got := isMockExempt(overrides, tt.file)
		if got != tt.exempt {
			t.Errorf("isMockExempt(%q) = %v, want %v", tt.file, got, tt.exempt)
		}
	}

	// Nil overrides.
	if isMockExempt(nil, "any.go") {
		t.Error("isMockExempt with nil overrides should be false")
	}
}

// --- closestMock tests ---

func TestClosestMock(t *testing.T) {
	mocks := map[int]MockUsage{
		5:  {Line: 5, Kind: "mock.New", Value: "mock.New()"},
		10: {Line: 10, Kind: "httptest", Value: "httptest."},
		20: {Line: 20, Kind: "gomock", Value: "gomock."},
	}

	tests := []struct {
		target int
		want   int
	}{
		{5, 5},
		{7, 5},  // closer to 5 than 10
		{12, 10}, // closer to 10
		{19, 20}, // closer to 20
		{30, 20}, // only 20 is closest
		{1, 5},   // closest is 5
	}

	for _, tt := range tests {
		got := closestMock(mocks, tt.target)
		if got.Line != tt.want {
			t.Errorf("closestMock(target=%d) line=%d, want %d", tt.target, got.Line, tt.want)
		}
	}
}

// --- deduplicate mock usages tests ---

func TestDedupeMockUsages(t *testing.T) {
	usages := []MockUsage{
		{File: "a_test.go", Line: 5, Kind: "mock.New"},
		{File: "a_test.go", Line: 10, Kind: "mock.New"},
		{File: "a_test.go", Line: 15, Kind: "httptest"},
		{File: "b_test.go", Line: 3, Kind: "mock.New"},
	}

	deduped := dedupeMockUsages(usages)
	if len(deduped) != 3 {
		t.Fatalf("len=%d, want 3", len(deduped))
	}

	// Should have kept first per file+kind.
	found := make(map[string]int)
	for _, u := range deduped {
		key := u.File + "|" + u.Kind
		found[key] = u.Line
	}

	if found["a_test.go|mock.New"] != 5 {
		t.Errorf("expected first mock.New at line 5, got %d", found["a_test.go|mock.New"])
	}
}

// --- MockReport JSON serialisation ---

func TestMockReportJSON(t *testing.T) {
	r := &MockReport{
		Slice:   "S68-lint-mock",
		Release: "test-release",
		MockUsages: []MockUsage{
			{File: "test_test.go", Line: 10, Kind: "mock.New", Value: "mock.New(ctrl)"},
		},
		Violations: []MockViolation{
			{
				File:      "test_test.go",
				Line:      10,
				MockKind:  "mock.New",
				MockValue: "mock.New(ctrl)",
				InfraKind: "DATABASE_URL",
				InfraLine: 15,
				InfraRef:  "DATABASE_URL",
				Msg:       "undeclared mock boundary",
			},
		},
		TotalViolations: 1,
		Verdict:         "FAIL",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed MockReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.TotalViolations != 1 {
		t.Errorf("TotalViolations=%d, want 1", parsed.TotalViolations)
	}
	if parsed.Verdict != "FAIL" {
		t.Errorf("Verdict=%q, want FAIL", parsed.Verdict)
	}
}

// --- PrintMock tests ---

func TestPrintMockPass(t *testing.T) {
	r := &MockReport{
		Slice:           "S68-lint-mock",
		Release:         "test-release",
		MockUsages:      []MockUsage{},
		Violations:      nil,
		TotalViolations: 0,
		Verdict:         "PASS",
	}

	out := PrintMock(r)
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS in output, got: %s", out)
	}
	if !strings.Contains(out, "No undeclared mock boundaries") {
		t.Errorf("expected 'No undeclared mock boundaries' in output")
	}
}

func TestPrintMockFail(t *testing.T) {
	r := &MockReport{
		Slice:   "S68-lint-mock",
		Release: "test-release",
		MockUsages: []MockUsage{
			{File: "test_test.go", Line: 10, Kind: "mock.New", Value: "mock.New(ctrl)"},
		},
		Violations: []MockViolation{
			{
				File:      "test_test.go",
				Line:      10,
				MockKind:  "mock.New",
				MockValue: "mock.New(ctrl)",
				InfraKind: "DATABASE_URL",
				InfraLine: 15,
				InfraRef:  "DATABASE_URL",
				Msg:       "undeclared mock boundary: mock.New in test file alongside DATABASE_URL reference",
			},
		},
		TotalViolations: 1,
		Verdict:         "FAIL",
	}

	out := PrintMock(r)
	if !strings.Contains(out, "FAIL") {
		t.Errorf("expected FAIL in output, got: %s", out)
	}
	if !strings.Contains(out, "Undeclared mock boundaries: 1") {
		t.Errorf("expected 'Undeclared mock boundaries: 1' in output")
	}
}

// --- JSONMock tests ---

func TestJSONMock(t *testing.T) {
	r := &MockReport{
		Slice:           "test",
		Release:         "r",
		TotalViolations: 0,
		Verdict:         "PASS",
	}

	out := JSONMock(r)
	if !strings.Contains(out, `"verdict": "PASS"`) {
		t.Errorf("expected PASS verdict in JSON: %s", out)
	}

	// Should parse.
	var parsed MockReport
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Errorf("failed to parse JSONMock output: %v", err)
	}
}