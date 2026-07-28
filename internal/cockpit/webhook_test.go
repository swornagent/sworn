package cockpit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
)

type fakeWebhookResolver struct {
	addresses []net.IPAddr
	err       error
	calls     int
}

func (f *fakeWebhookResolver) LookupIPAddr(
	context.Context,
	string,
) ([]net.IPAddr, error) {
	f.calls++
	return append([]net.IPAddr(nil), f.addresses...), f.err
}

type fakeWebhookDialer struct {
	addresses []string
	err       error
}

func (f *fakeWebhookDialer) DialContext(
	_ context.Context,
	_, address string,
) (net.Conn, error) {
	f.addresses = append(f.addresses, address)
	return nil, f.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

type trackingBody struct {
	body   *bytes.Reader
	closed bool
}

func newTrackingBody(value string) *trackingBody {
	return &trackingBody{body: bytes.NewReader([]byte(value))}
}

func (b *trackingBody) Read(value []byte) (int, error) {
	return b.body.Read(value)
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

func cockpitJournalFixture(
	t *testing.T,
) (*journal.Store, journal.Run) {
	t.Helper()
	ctx := context.Background()
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "operator.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_200_000, 0).UTC()
	run := journal.Run{
		ID: "run-1", ManifestDigest: testDigest("manifest"),
		Repository: "/private/repository", Release: "release-1",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	return store, run
}

func testDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func testWebhookDestinations() []WebhookDestination {
	return []WebhookDestination{
		{
			ID: "primary", URL: "https://hooks.example.com/sworn",
			Secret: []byte("primary-secret-is-at-least-32-bytes"),
		},
		{
			ID: "audit", URL: "https://audit.example.com/sworn",
			Secret: []byte("audit-secret-is-also-at-least-32"),
		},
	}
}

func testWebhookService(
	t *testing.T,
	store webhookJournal,
	destinations []WebhookDestination,
) *WebhookService {
	t.Helper()
	service, err := newWebhookService(
		store,
		destinations,
		&fakeWebhookResolver{addresses: []net.IPAddr{{
			IP: net.ParseIP("8.8.8.8"),
		}}},
		&fakeWebhookDialer{err: errors.New("not dialed in unit test")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestWebhookProjectsCanonicalContentFreeEventsPerDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run := cockpitJournalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	for index, kind := range []string{"effect_started", "effect_completed"} {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			kind,
			[]byte("private prompt, proof body, and /private/source/path"),
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	destinations := testWebhookDestinations()
	service := testWebhookService(t, store, destinations)
	service.now = func() time.Time { return now.Add(3 * time.Second) }

	for _, destination := range destinations {
		result, err := service.Project(ctx, run.ID, destination.ID)
		if err != nil {
			t.Fatal(err)
		}
		if result.Projected != 2 || result.ThroughOffset < 2 ||
			result.HasMore {
			t.Fatalf("%s projection = %#v", destination.ID, result)
		}
		notifications, err := store.Notifications(
			ctx,
			run.ID,
			destination.ID,
			journal.MaxObserverItems,
		)
		if err != nil || len(notifications) != 2 {
			t.Fatalf(
				"%s notifications = %#v, %v",
				destination.ID,
				notifications,
				err,
			)
		}
		for index, item := range notifications {
			for _, private := range []string{
				"private prompt",
				"proof body",
				"/private/source/path",
				run.Repository,
			} {
				if bytes.Contains(item.Body, []byte(private)) {
					t.Errorf(
						"%s body contains private value %q",
						destination.ID,
						private,
					)
				}
			}
			var body webhookEventBody
			if err := json.Unmarshal(item.Body, &body); err != nil {
				t.Fatal(err)
			}
			if body.SchemaVersion != WebhookEventSchemaVersion ||
				body.DestinationID != destination.ID ||
				body.DestinationBinding !=
					service.destinations[destination.ID].binding ||
				body.MessageID != item.MessageID ||
				body.RunID != run.ID ||
				body.EventOffset != item.SourceEventOffset ||
				body.EventKind !=
					[]string{"effect_started", "effect_completed"}[index] {
				t.Fatalf("%s body = %#v", destination.ID, body)
			}
			canonical, err := json.Marshal(body)
			if err != nil || !bytes.Equal(canonical, item.Body) {
				t.Fatalf(
					"%s body is not canonical: %s, %v",
					destination.ID,
					item.Body,
					err,
				)
			}
		}
		replay, err := service.Project(ctx, run.ID, destination.ID)
		if err != nil || replay.Projected != 0 ||
			replay.ThroughOffset != result.ThroughOffset {
			t.Fatalf("%s replay = %#v, %v", destination.ID, replay, err)
		}
	}
	primary, _ := store.Notifications(
		ctx,
		run.ID,
		"primary",
		journal.MaxObserverItems,
	)
	audit, _ := store.Notifications(
		ctx,
		run.ID,
		"audit",
		journal.MaxObserverItems,
	)
	if primary[0].MessageID == audit[0].MessageID {
		t.Fatal("per-destination message identities collided")
	}
}

func TestWebhookSignsStoredBytesAndFinishesOutsideRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run := cockpitJournalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	if err := store.AppendEvent(
		ctx,
		run.ID,
		"effect_completed",
		[]byte("private event body"),
		now,
	); err != nil {
		t.Fatal(err)
	}
	destination := testWebhookDestinations()[0]
	service := testWebhookService(
		t,
		store,
		[]WebhookDestination{destination},
	)
	service.now = func() time.Time { return now.Add(time.Second) }
	if _, err := service.Project(ctx, run.ID, destination.ID); err != nil {
		t.Fatal(err)
	}
	before, err := store.Notifications(
		ctx,
		run.ID,
		destination.ID,
		1,
	)
	if err != nil || len(before) != 1 {
		t.Fatalf("pending notification = %#v, %v", before, err)
	}
	var requestBody []byte
	headers := make(http.Header)
	responseBody := newTrackingBody("response must never persist")
	service.destinations[destination.ID].client.Transport = roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			requestBody, err = io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			headers = request.Header.Clone()
			if request.URL.String() != destination.URL ||
				request.Method != http.MethodPost ||
				request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("request = %s %s %#v", request.Method, request.URL, request.Header)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       responseBody,
			}, nil
		},
	)
	result, err := service.DeliverOne(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.State != journal.NotificationDelivered ||
		result.RunID != run.ID ||
		result.MessageID != before[0].MessageID ||
		result.LastError != "" {
		t.Fatalf("delivery = %#v", result)
	}
	if !bytes.Equal(requestBody, before[0].Body) ||
		headers.Get("X-Sworn-Message-ID") != before[0].MessageID {
		t.Fatalf("signed request identity/body mismatch")
	}
	endpoint := service.destinations[destination.ID]
	signedItem := before[0]
	signedItem.Attempts = 1
	mac := hmac.New(sha256.New, destination.Secret)
	_, _ = mac.Write(webhookSignatureBytes(
		endpoint,
		signedItem,
		now.Add(time.Second),
	))
	wantSignature := "hmac-sha256=" + hex.EncodeToString(mac.Sum(nil))
	if headers.Get("X-Sworn-Signature") != wantSignature ||
		headers.Get("X-Sworn-Signature-Version") != "v1" ||
		headers.Get("X-Sworn-Destination-ID") != destination.ID ||
		headers.Get("X-Sworn-Attempt") != "1" ||
		headers.Get("X-Sworn-Sent-At") !=
			now.Add(time.Second).Format(time.RFC3339Nano) {
		t.Fatalf("signature headers = %#v, want signature %q", headers, wantSignature)
	}
	if !responseBody.closed {
		t.Fatal("response body was not closed")
	}
	after, err := store.Notifications(
		ctx,
		run.ID,
		destination.ID,
		1,
	)
	if err != nil || after[0].State != journal.NotificationDelivered ||
		!bytes.Equal(after[0].Body, before[0].Body) ||
		bytes.Contains(after[0].Body, []byte("response must never persist")) {
		t.Fatalf("delivered notification = %#v, %v", after, err)
	}
	empty, err := service.DeliverOne(ctx, destination.ID)
	if err != nil || empty.Found {
		t.Fatalf("empty delivery = %#v, %v", empty, err)
	}
}

func TestWebhookRetriesWithFixedCodesAndNeverStoresResponses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run := cockpitJournalFixture(t)
	current := run.CreatedAt.Add(time.Second)
	if err := store.AppendEvent(
		ctx,
		run.ID,
		"effect_failed",
		[]byte("private"),
		current,
	); err != nil {
		t.Fatal(err)
	}
	destination := testWebhookDestinations()[0]
	service := testWebhookService(
		t,
		store,
		[]WebhookDestination{destination},
	)
	service.now = func() time.Time { return current }
	if _, err := service.Project(ctx, run.ID, destination.ID); err != nil {
		t.Fatal(err)
	}
	status := http.StatusServiceUnavailable
	service.destinations[destination.ID].client.Transport = roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       newTrackingBody("provider response secret"),
			}, nil
		},
	)
	first, err := service.DeliverOne(ctx, destination.ID)
	if err != nil || first.State != journal.NotificationPending ||
		first.LastError != "WEBHOOK_HTTP_RETRYABLE" ||
		first.Attempts != 1 {
		t.Fatalf("first delivery = %#v, %v", first, err)
	}
	current = current.Add(webhookRetryBase)
	status = http.StatusBadRequest
	second, err := service.DeliverOne(ctx, destination.ID)
	if err != nil || second.State != journal.NotificationDead ||
		second.LastError != "WEBHOOK_HTTP_REJECTED" ||
		second.Attempts != 2 {
		t.Fatalf("second delivery = %#v, %v", second, err)
	}
	items, err := store.Notifications(
		ctx,
		run.ID,
		destination.ID,
		1,
	)
	if err != nil || items[0].State != journal.NotificationDead ||
		items[0].LastErrorCode != "WEBHOOK_HTTP_REJECTED" ||
		bytes.Contains(items[0].Body, []byte("provider response secret")) {
		t.Fatalf("terminal outbox item = %#v, %v", items, err)
	}
}

func TestWebhookNeverReroutesQueuedRowsAfterConfigChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run := cockpitJournalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	if err := store.AppendEvent(
		ctx,
		run.ID,
		"effect_completed",
		[]byte("private"),
		now,
	); err != nil {
		t.Fatal(err)
	}
	original := testWebhookDestinations()[0]
	first := testWebhookService(
		t,
		store,
		[]WebhookDestination{original},
	)
	first.now = func() time.Time { return now.Add(time.Second) }
	if _, err := first.Project(ctx, run.ID, original.ID); err != nil {
		t.Fatal(err)
	}

	changed := original
	changed.URL = "https://replacement.example.com/sworn"
	changed.Secret = []byte("replacement-secret-is-at-least-32-bytes")
	second := testWebhookService(
		t,
		store,
		[]WebhookDestination{changed},
	)
	second.now = func() time.Time { return now.Add(2 * time.Second) }
	sent := 0
	second.destinations[changed.ID].client.Transport = roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			sent++
			return nil, errors.New("must not send")
		},
	)
	result, err := second.DeliverOne(ctx, changed.ID)
	if err != nil || !result.Found ||
		result.State != journal.NotificationDead ||
		result.LastError != "WEBHOOK_CONFIG_CHANGED" ||
		sent != 0 {
		t.Fatalf(
			"changed config delivery = %#v, sent=%d, %v",
			result,
			sent,
			err,
		)
	}
}

func TestWebhookTickProjectsAndDrainsDestinationsInOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run := cockpitJournalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	if err := store.AppendEvent(
		ctx,
		run.ID,
		"effect_completed",
		[]byte("private"),
		now,
	); err != nil {
		t.Fatal(err)
	}
	destinations := testWebhookDestinations()
	service := testWebhookService(t, store, destinations)
	service.now = func() time.Time { return now.Add(time.Second) }
	var delivered []string
	for _, destination := range destinations {
		destinationID := destination.ID
		service.destinations[destinationID].client.Transport = roundTripFunc(
			func(*http.Request) (*http.Response, error) {
				delivered = append(delivered, destinationID)
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       http.NoBody,
				}, nil
			},
		)
	}
	result, err := service.Tick(ctx, run.ID)
	if err != nil || result.Projected != 2 ||
		result.Deliveries != 2 || result.HasMore {
		t.Fatalf("tick = %#v, %v", result, err)
	}
	if strings.Join(delivered, ",") != "audit,primary" {
		t.Fatalf("delivery order = %#v", delivered)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Run(
		cancelled,
		run.ID,
		minWebhookInterval,
	); err != nil {
		t.Fatalf("cancelled worker = %v", err)
	}
}

func TestWebhookDestinationAndDialPolicyFailClosed(t *testing.T) {
	t.Parallel()

	store, _ := cockpitJournalFixture(t)
	secret := []byte("destination-secret-is-at-least-32")
	tests := []WebhookDestination{
		{ID: "bad", URL: "http://hooks.example.com/path", Secret: secret},
		{ID: "bad", URL: "https://127.0.0.1/path", Secret: secret},
		{ID: "bad", URL: "https://localhost/path", Secret: secret},
		{ID: "bad", URL: "https://hooks.example.com/path?token=x", Secret: secret},
		{ID: "bad", URL: "https://hooks.example.com/../admin", Secret: secret},
		{ID: "bad", URL: "https://hooks.example.com/a%2fb", Secret: secret},
		{ID: "bad", URL: "https://hooks.example.com/path", Secret: []byte("short")},
		{ID: "bad/id", URL: "https://hooks.example.com/path", Secret: secret},
		{ID: "bad:id", URL: "https://hooks.example.com/path", Secret: secret},
		{
			ID:  strings.Repeat("a", 121),
			URL: "https://hooks.example.com/path", Secret: secret,
		},
	}
	for _, destination := range tests {
		if _, err := newWebhookService(
			store,
			[]WebhookDestination{destination},
			&fakeWebhookResolver{},
			&fakeWebhookDialer{},
		); !IsCode(err, "INVALID_WEBHOOK_DESTINATION") {
			t.Errorf("%q admitted: %v", destination.URL, err)
		}
	}
	valid := WebhookDestination{
		ID: "same", URL: "https://hooks.example.com",
		Secret: secret,
	}
	if _, err := newWebhookService(
		store,
		[]WebhookDestination{valid, valid},
		&fakeWebhookResolver{},
		&fakeWebhookDialer{},
	); !IsCode(err, "DUPLICATE_WEBHOOK_DESTINATION") {
		t.Fatalf("duplicate destination = %v", err)
	}
	admitted, err := newWebhookService(
		store,
		[]WebhookDestination{valid},
		&fakeWebhookResolver{},
		&fakeWebhookDialer{},
	)
	if err != nil || admitted.destinations["same"].url.Path != "/" {
		t.Fatalf("empty path normalization = %#v, %v", admitted, err)
	}
	if _, err := admitted.Project(
		context.Background(),
		"bad/run",
		"same",
	); !IsCode(err, "INVALID_WEBHOOK_REQUEST") {
		t.Fatalf("broad run identity admitted = %v", err)
	}

	resolver := &fakeWebhookResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("8.8.8.8")},
		{IP: net.ParseIP("127.0.0.1")},
	}}
	dialer := &fakeWebhookDialer{}
	safe := &safeWebhookDialer{
		host: "hooks.example.com", port: "443",
		resolver: resolver, dialer: dialer,
	}
	_, err = safe.DialContext(
		context.Background(),
		"tcp",
		"hooks.example.com:443",
	)
	var networkError *webhookNetworkError
	if !errors.As(err, &networkError) ||
		networkError.code != "WEBHOOK_DNS_REJECTED" ||
		len(dialer.addresses) != 0 {
		t.Fatalf(
			"mixed public/private DNS = %v, dialed %#v",
			err,
			dialer.addresses,
		)
	}
	if _, err := safe.DialContext(
		context.Background(),
		"tcp",
		"other.example.com:443",
	); !errors.As(err, &networkError) ||
		networkError.code != "WEBHOOK_AUTHORITY_CHANGED" {
		t.Fatalf("changed authority = %v", err)
	}

	for address, want := range map[string]bool{
		"8.8.8.8":         true,
		"2606:4700::1111": true,
		"127.0.0.1":       false,
		"10.0.0.1":        false,
		"169.254.169.254": false,
		"100.64.0.1":      false,
		"64:ff9b::7f00:1": false,
		"2001:db8::1":     false,
	} {
		if got := publicWebhookIP(netip.MustParseAddr(address)); got != want {
			t.Errorf("publicWebhookIP(%s) = %t, want %t", address, got, want)
		}
	}
}

func TestWebhookRedirectsAreNeverFollowed(t *testing.T) {
	t.Parallel()

	store, _ := cockpitJournalFixture(t)
	service := testWebhookService(
		t,
		store,
		[]WebhookDestination{testWebhookDestinations()[0]},
	)
	client := service.destinations["primary"].client
	err := client.CheckRedirect(
		&http.Request{},
		[]*http.Request{{}},
	)
	if !errors.Is(err, errWebhookRedirect) {
		t.Fatalf("redirect policy = %v", err)
	}
	code, permanent := classifyWebhookError(
		context.Background(),
		&urlErrorForTest{err: errWebhookRedirect},
	)
	if code != "WEBHOOK_REDIRECT" || !permanent {
		t.Fatalf("redirect classification = %s permanent=%t", code, permanent)
	}
}

func TestWebhookSignatureCanonicalGoldenAndTamperBinding(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	endpoint, err := prepareWebhookEndpoint(
		WebhookDestination{
			ID: "primary", URL: "https://hooks.example.com:8443/v1",
			Secret: secret,
		},
		&fakeWebhookResolver{},
		&fakeWebhookDialer{},
	)
	if err != nil {
		t.Fatal(err)
	}
	item := journal.Notification{
		MessageID: "message-1",
		Attempts:  2,
		Body:      []byte(`{"safe":true}`),
	}
	sentAt := time.Date(
		2023,
		time.November,
		17,
		5,
		46,
		40,
		123456789,
		time.UTC,
	)
	canonical := webhookSignatureBytes(endpoint, item, sentAt)
	const wantCanonical = `{"version":"v1","method":"POST",` +
		`"authority":"hooks.example.com:8443","path":"/v1",` +
		`"destination_id":"primary",` +
		`"destination_binding":"sha256:e0822587306449e2ef5621a098ea951aad54e4048e78bdd8be927c58383c09ff",` +
		`"message_id":"message-1","attempt":2,` +
		`"sent_at":"2023-11-17T05:46:40.123456789Z",` +
		`"body_digest":"sha256:13f513fe32a8991557ebf28941b75597641e94717c08569b7723d998c7428423"}`
	if string(canonical) != wantCanonical {
		t.Fatalf("canonical signature bytes = %s", canonical)
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(canonical)
	const wantHMAC = "5f0e5e97b1161c59f32b6528c535a0f8f4f8650911162fd9acb07fd367bc7c56"
	if got := hex.EncodeToString(mac.Sum(nil)); got != wantHMAC {
		t.Fatalf("golden HMAC = %s, want %s", got, wantHMAC)
	}

	tampered := *endpoint
	tampered.binding = testDigest("changed-binding")
	changed := webhookSignatureBytes(&tampered, item, sentAt)
	if bytes.Equal(changed, canonical) {
		t.Fatal("destination binding tamper did not change canonical bytes")
	}
	tamperedMAC := hmac.New(sha256.New, secret)
	_, _ = tamperedMAC.Write(changed)
	if hmac.Equal(mac.Sum(nil), tamperedMAC.Sum(nil)) {
		t.Fatal("destination binding tamper retained signature")
	}
}

type urlErrorForTest struct {
	err error
}

func (e *urlErrorForTest) Error() string {
	return "redacted"
}

func (e *urlErrorForTest) Unwrap() error {
	return e.err
}

func TestWebhookPayloadVocabularyStaysContentFree(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(webhookEventBody{})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"prompt",
		"completion",
		"body",
		"path",
		"repository",
		"proof",
		"model",
		"error",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Errorf("webhook schema contains forbidden field %q: %s", forbidden, body)
		}
	}
}
