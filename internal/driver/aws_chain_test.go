//go:build linux

package driver

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const awsEnvironmentTable = `NAME       : VALUE                    : TYPE             : LOCATION
profile    : <not set>                : None             : None
access_key : ****************MPLE     : env              :
secret_key : ****************alue     : env              :
region     : ap-southeast-2           : env              : ['AWS_REGION', 'AWS_DEFAULT_REGION']
`

func TestAWSConfigureListBindsExact2359GrammarAndSourceTruth(t *testing.T) {
	t.Parallel()
	snapshot, err := parseAWSConfigureList([]byte(awsEnvironmentTable))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Profile != "" || snapshot.Region != "ap-southeast-2" ||
		snapshot.RegionSource != AWSSourceEnvironment ||
		snapshot.CredentialSource != AWSSourceEnvironment ||
		snapshot.sourceFingerprint == ([32]byte{}) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	configTable := `NAME       : VALUE                    : TYPE                     : LOCATION
profile    : production               : manual                   : --profile
access_key : ****************MPLE     : shared-credentials-file  : /private/aws/credentials
secret_key : ****************alue     : shared-credentials-file  : /private/aws/credentials
region     : ap-southeast-2           : config-file              : /private/aws/config
`
	configSnapshot, err := parseAWSConfigureList([]byte(configTable))
	if err != nil {
		t.Fatal(err)
	}
	if configSnapshot.Profile != "production" ||
		configSnapshot.RegionSource != AWSSourceSharedFiles ||
		configSnapshot.CredentialSource != AWSSourceSharedFiles {
		t.Fatalf("config snapshot = %#v", configSnapshot)
	}
	for _, mutation := range [][]byte{
		bytes.Replace([]byte(awsEnvironmentTable), []byte("NAME"), []byte("Name"), 1),
		bytes.Replace([]byte(awsEnvironmentTable), []byte("env              :\nsecret"), []byte("env              : source\nsecret"), 1),
		bytes.Replace([]byte(awsEnvironmentTable), []byte("AWS_REGION"), []byte("OTHER_REGION"), 1),
		append([]byte(awsEnvironmentTable), []byte("extra : row : env : x\n")...),
	} {
		if _, err := parseAWSConfigureList(mutation); !IsCode(err, "AWS_NOT_CERTIFIED") {
			t.Fatalf("mutation accepted: %q, %v", mutation, err)
		}
	}
}

func TestAWSExportParserIsBoundedMutableClosedAndCleared(t *testing.T) {
	t.Parallel()
	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	body := []byte(`{"Version":1,"AccessKeyId":"AKIAEXAMPLE1234","SecretAccessKey":"secret-example-value","SessionToken":"session-example-value","Expiration":"` + expiration + `"}`)
	credentials, err := parseAWSExport(body, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	accessAlias := credentials.accessKeyID
	secretAlias := credentials.secretAccessKey
	tokenAlias := credentials.sessionToken
	if string(accessAlias) != "AKIAEXAMPLE1234" ||
		string(secretAlias) != "secret-example-value" ||
		string(tokenAlias) != "session-example-value" {
		t.Fatal("credential parser changed values")
	}
	credentials.Close()
	for name, value := range map[string][]byte{
		"access": accessAlias, "secret": secretAlias, "token": tokenAlias,
	} {
		if !bytes.Equal(value, make([]byte, len(value))) {
			t.Fatalf("%s buffer not best-effort cleared: %q", name, value)
		}
	}
	for _, invalid := range [][]byte{
		[]byte(`{"Version":1,"AccessKeyId":"AKIAEXAMPLE1234","AccessKeyId":"duplicate","SecretAccessKey":"secret-example-value"}`),
		[]byte(`{"Version":2,"AccessKeyId":"AKIAEXAMPLE1234","SecretAccessKey":"secret-example-value"}`),
		[]byte(`{"Version":1,"AccessKeyId":"AKIAEXAMPLE1234","SecretAccessKey":"secret-example-value","Unknown":"x"}`),
		[]byte(`{"Version":1,"AccessKeyId":"short","SecretAccessKey":"short"}`),
	} {
		if _, err := parseAWSExport(invalid, time.Now().UTC()); !IsCode(err, "AWS_CREDENTIAL_EXPORT_INVALID") {
			t.Fatalf("invalid export accepted: %s, %v", invalid, err)
		}
	}
	oversized := bytes.Repeat([]byte("x"), MaxAWSExportBytes+1)
	if _, err := parseAWSExport(oversized, time.Now().UTC()); !IsCode(err, "AWS_CREDENTIAL_EXPORT_INVALID") {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestAWSChainDoubleSnapshotRejectsDriftAndConflictingRegion(t *testing.T) {
	t.Parallel()
	awsPath := "/usr/local/aws-cli/v2/2.35.9/dist/aws"
	if _, err := os.Stat(awsPath); err != nil {
		t.Skip("exact AWS CLI fixture unavailable")
	}
	spec := AWSChainSpec{
		CLI:        ExecutableIdentity{Path: awsPath, Digest: AWSCLIDigest},
		CLIVersion: AWSCLIVersion,
		Region:     "ap-southeast-2", RegionSource: AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			"AWS_REGION", "AWS_DEFAULT_REGION",
		},
		RuntimeFiles: awsRuntimeIdentityFixture(),
		RequiredRuntimeTargets: []string{
			"/etc/ssl/certs/ca-certificates.crt",
			"/etc/resolv.conf",
			"/etc/hosts",
			"/etc/nsswitch.conf",
		},
	}
	environment := func(defaultRegion string) [][]byte {
		return [][]byte{
			[]byte("AWS_ACCESS_KEY_ID=AKIAEXAMPLE1234"),
			[]byte("AWS_SECRET_ACCESS_KEY=secret-example-value"),
			[]byte("AWS_REGION=ap-southeast-2"),
			[]byte("AWS_DEFAULT_REGION=" + defaultRegion),
		}
	}
	expiration := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	export := []byte(`{"Version":1,"AccessKeyId":"AKIAEXAMPLE1234","SecretAccessKey":"secret-example-value","Expiration":"` + expiration + `"}`)
	calls := 0
	runner := func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		arguments ...string,
	) ([]byte, error) {
		calls++
		if strings.Join(arguments, " ") == "configure export-credentials --format process" {
			return append([]byte(nil), export...), nil
		}
		return []byte(awsEnvironmentTable), nil
	}
	snapshot, credentials, err := resolveAWSChain(
		context.Background(), spec, environment("ap-southeast-2"), runner,
	)
	if err != nil || calls != 3 || snapshot.Region != spec.Region {
		t.Fatalf("resolve = %#v, calls=%d, err=%v", snapshot, calls, err)
	}
	credentials.Close()
	calls = 0
	if _, _, err := resolveAWSChain(
		context.Background(), spec, environment("us-east-1"), runner,
	); !IsCode(err, "AWS_NOT_CERTIFIED") || calls != 0 {
		t.Fatalf("conflicting region = calls %d, error %v", calls, err)
	}
	driftedTable := strings.Replace(
		awsEnvironmentTable,
		"['AWS_REGION', 'AWS_DEFAULT_REGION']",
		"['AWS_DEFAULT_REGION', 'AWS_REGION']",
		1,
	)
	calls = 0
	driftRunner := func(
		_ context.Context,
		_ AWSChainSpec,
		_ [][]byte,
		arguments ...string,
	) ([]byte, error) {
		calls++
		if strings.Contains(strings.Join(arguments, " "), "export-credentials") {
			return append([]byte(nil), export...), nil
		}
		if calls == 3 {
			return []byte(driftedTable), nil
		}
		return []byte(awsEnvironmentTable), nil
	}
	if _, _, err := resolveAWSChain(
		context.Background(), spec, environment("ap-southeast-2"), driftRunner,
	); !IsCode(err, "AWS_NOT_CERTIFIED") {
		t.Fatalf("source-location drift error = %v", err)
	}
}

func awsRuntimeIdentityFixture() []PinnedRuntimeFile {
	const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	targets := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	}
	files := make([]PinnedRuntimeFile, len(targets))
	for index, target := range targets {
		files[index] = PinnedRuntimeFile{
			Path: target, Target: target, Digest: digest,
		}
	}
	return files
}

func TestSigV4VectorIsDeterministicAndBindsBodyRegionAndToken(t *testing.T) {
	t.Parallel()
	credentials := &awsCredentials{
		accessKeyID:     []byte("AKIDEXAMPLE"),
		secretAccessKey: []byte("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"),
		sessionToken:    []byte("session-token"),
	}
	defer credentials.Close()
	request, err := http.NewRequest(
		http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/example/converse",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	body := []byte(`{"messages":[]}`)
	now := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	if err := signAWSRequest(request, body, credentials, "us-east-1", "bedrock", now); err != nil {
		t.Fatal(err)
	}
	const expected = "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/bedrock/aws4_request, SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date;x-amz-security-token, Signature=2577f5bf9d98142e79160570d87a9d3e0339a70190fc2a72aa12aa07151029ae"
	if request.Header.Get("Authorization") != expected ||
		request.Header.Get("X-Amz-Date") != "20150830T123600Z" ||
		request.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Fatalf("signed headers = %#v", request.Header)
	}
	first := request.Header.Get("Authorization")
	request2, _ := http.NewRequest(
		http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/example/converse",
		nil,
	)
	request2.Header.Set("Content-Type", "application/json")
	if err := signAWSRequest(request2, []byte(`{"messages":[1]}`), credentials, "us-east-1", "bedrock", now); err != nil {
		t.Fatal(err)
	}
	if request2.Header.Get("Authorization") == first {
		t.Fatal("body mutation did not change signature")
	}
}

func TestAWSCLIUsesOnlyExactPinnedClosureAndClosedCommands(t *testing.T) {
	t.Parallel()
	const awsPath = "/usr/local/aws-cli/v2/2.35.9/dist/aws"
	if _, err := os.Stat(awsPath); err != nil {
		t.Skip("exact AWS CLI fixture unavailable")
	}
	spec := AWSChainSpec{
		CLI:              ExecutableIdentity{Path: awsPath, Digest: AWSCLIDigest},
		CLIVersion:       AWSCLIVersion,
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION",
		},
		RuntimeFiles: awsRuntimeIdentityFixture(),
		RequiredRuntimeTargets: []string{
			"/etc/ssl/certs/ca-certificates.crt",
			"/etc/resolv.conf",
			"/etc/hosts",
			"/etc/nsswitch.conf",
		},
	}
	arguments, err := awsBubblewrapArguments(
		spec,
		4+len(spec.RuntimeFiles),
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\x00")
	if !slicesContain(arguments, "--die-with-parent") ||
		!slicesContain(arguments, "--share-net") ||
		!strings.Contains(joined, "\x003\x00"+awsPath) ||
		arguments[len(arguments)-1] != awsPath {
		t.Fatalf("AWS bwrap arguments = %q", arguments)
	}
	for index := 0; index < len(arguments)-1; index++ {
		if arguments[index] == "--ro-bind" &&
			(arguments[index+1] == "/usr" || arguments[index+1] == "/") {
			t.Fatalf("ambient host bind = %q", arguments[index:index+2])
		}
	}
	for _, invalid := range [][]string{
		{"s3", "ls"},
		{"configure", "list", "--debug"},
		{"configure", "export-credentials", "--format", "env"},
	} {
		if validAWSCommandArguments(spec.Profile, invalid) {
			t.Fatalf("AWS command accepted: %q", invalid)
		}
	}
}
