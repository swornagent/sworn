package cockpit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	WebhookEventSchemaVersion = "sworn.webhook-event/v1"
	webhookObserverPrefix     = "webhook."
	webhookProjectionLimit    = 128
	webhookClaimLease         = 30 * time.Second
	webhookRequestTimeout     = 10 * time.Second
	webhookFinishTimeout      = 5 * time.Second
	webhookRetryBase          = 5 * time.Second
	maxWebhookAddresses       = 16
	minWebhookInterval        = 250 * time.Millisecond
	maxWebhookInterval        = time.Minute
)

const (
	webhookRunUpdated      = "run_updated"
	webhookControlUpdated  = "control_updated"
	webhookAttemptUpdated  = "attempt_updated"
	webhookRecoveryUpdated = "recovery_updated"
)

var (
	webhookHostnamePattern = regexp.MustCompile(
		`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?` +
			`(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)$`,
	)
	webhookPathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)
	webhookRunPattern  = regexp.MustCompile(
		`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`,
	)
	webhookDestinationPattern = regexp.MustCompile(
		`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`,
	)
	errWebhookRedirect  = errors.New("webhook redirect rejected")
	webhookDeniedRanges = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/128"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("2001:2::/48"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("ff00::/8"),
		netip.MustParsePrefix("64:ff9b::/96"),
	}
)

// WebhookDestination IDs are immutable bindings. Changing a URL or secret for
// an existing ID dead-letters already-queued rows instead of rerouting them.
type WebhookDestination struct {
	ID     string
	URL    string
	Secret []byte
}

type WebhookProjection struct {
	DestinationID string
	Projected     int
	ThroughOffset int64
	HasMore       bool
}

type WebhookDelivery struct {
	Found       bool
	RunID       string
	MessageID   string
	Attempts    int64
	State       journal.NotificationState
	LastError   string
	DeliveredAt time.Time
}

type WebhookTick struct {
	Projected  int
	Deliveries int
	HasMore    bool
}

type webhookJournal interface {
	ObserverCursor(context.Context, string, string) (int64, error)
	EventsAfter(context.Context, string, int64, int) (journal.EventWindow, error)
	AdvanceObserver(context.Context, journal.ObserverAdvance) error
	ClaimNotification(
		context.Context,
		string,
		time.Time,
		time.Duration,
	) (journal.NotificationClaim, bool, error)
	FinishNotification(
		context.Context,
		journal.NotificationClaim,
		journal.NotificationDisposition,
		time.Time,
		string,
		time.Time,
	) error
}

type webhookResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type webhookDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type webhookEndpoint struct {
	id      string
	url     *url.URL
	binding string
	secret  []byte
	client  *http.Client
}

type WebhookService struct {
	journal      webhookJournal
	destinations map[string]*webhookEndpoint
	now          func() time.Time
}

type webhookEventBody struct {
	SchemaVersion      string                           `json:"schema_version"`
	DestinationID      string                           `json:"destination_id"`
	DestinationBinding string                           `json:"destination_binding"`
	MessageID          string                           `json:"message_id"`
	RunID              string                           `json:"run_id"`
	EventOffset        int64                            `json:"event_offset"`
	EventKind          string                           `json:"event_kind"`
	RecordedAt         time.Time                        `json:"recorded_at"`
	CaptainDecision    *runtimepkg.CaptainDecisionEvent `json:"captain_decision,omitempty"`
}

type webhookDestinationIdentity struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	SecretDigest string `json:"secret_digest"`
}

type webhookSignatureInput struct {
	Version            string `json:"version"`
	Method             string `json:"method"`
	Authority          string `json:"authority"`
	Path               string `json:"path"`
	DestinationID      string `json:"destination_id"`
	DestinationBinding string `json:"destination_binding"`
	MessageID          string `json:"message_id"`
	Attempt            int64  `json:"attempt"`
	SentAt             string `json:"sent_at"`
	BodyDigest         string `json:"body_digest"`
}

type webhookNetworkError struct {
	code string
}

func (e *webhookNetworkError) Error() string {
	return e.code
}

func NewWebhookService(
	store webhookJournal,
	destinations []WebhookDestination,
) (*WebhookService, error) {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: -1,
	}
	return newWebhookService(
		store,
		destinations,
		net.DefaultResolver,
		dialer,
	)
}

func newWebhookService(
	store webhookJournal,
	destinations []WebhookDestination,
	resolver webhookResolver,
	dialer webhookDialer,
) (*WebhookService, error) {
	if store == nil || resolver == nil || dialer == nil ||
		len(destinations) == 0 || len(destinations) > 32 {
		return nil, fail("INVALID_WEBHOOK_CONFIG")
	}
	result := &WebhookService{
		journal:      store,
		destinations: make(map[string]*webhookEndpoint, len(destinations)),
		now:          time.Now,
	}
	for _, destination := range destinations {
		endpoint, err := prepareWebhookEndpoint(
			destination,
			resolver,
			dialer,
		)
		if err != nil {
			return nil, err
		}
		if _, duplicate := result.destinations[endpoint.id]; duplicate {
			return nil, fail("DUPLICATE_WEBHOOK_DESTINATION")
		}
		result.destinations[endpoint.id] = endpoint
	}
	return result, nil
}

func prepareWebhookEndpoint(
	destination WebhookDestination,
	resolver webhookResolver,
	dialer webhookDialer,
) (*webhookEndpoint, error) {
	if !webhookDestinationPattern.MatchString(destination.ID) ||
		len(destination.Secret) < 32 || len(destination.Secret) > 512 {
		return nil, fail("INVALID_WEBHOOK_DESTINATION")
	}
	parsed, err := url.Parse(destination.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" ||
		parsed.User != nil || parsed.Host == "" || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		len(parsed.String()) > 1024 {
		return nil, fail("INVALID_WEBHOOK_DESTINATION")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if len(hostname) > 253 || !webhookHostnamePattern.MatchString(hostname) ||
		net.ParseIP(hostname) != nil ||
		!webhookPathPattern.MatchString(parsed.Path) ||
		strings.Contains(parsed.Path, "//") ||
		hasDotPathSegment(parsed.Path) {
		return nil, fail("INVALID_WEBHOOK_DESTINATION")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	} else {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return nil, fail("INVALID_WEBHOOK_DESTINATION")
		}
	}
	parsed.Host = hostname
	if port != "443" {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	binding := webhookDestinationBinding(
		destination.ID,
		parsed.String(),
		destination.Secret,
	)
	safeDialer := &safeWebhookDialer{
		host: hostname, port: port,
		resolver: resolver,
		dialer:   dialer,
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            safeDialer.DialContext,
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxIdleConns:           0,
		IdleConnTimeout:        time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 8 * 1024,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: hostname,
		},
		TLSNextProto: map[string]func(
			string,
			*tls.Conn,
		) http.RoundTripper{},
	}
	return &webhookEndpoint{
		id:      destination.ID,
		url:     parsed,
		binding: binding,
		secret:  append([]byte(nil), destination.Secret...),
		client: &http.Client{
			Transport: transport,
			Timeout:   webhookRequestTimeout,
			CheckRedirect: func(
				*http.Request,
				[]*http.Request,
			) error {
				return errWebhookRedirect
			},
		},
	}, nil
}

func webhookDestinationBinding(
	id, destinationURL string,
	secret []byte,
) string {
	secretDigest := sha256.Sum256(secret)
	body, _ := json.Marshal(webhookDestinationIdentity{
		ID:  id,
		URL: destinationURL,
		SecretDigest: "sha256:" +
			hex.EncodeToString(secretDigest[:]),
	})
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hasDotPathSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func (s *WebhookService) Project(
	ctx context.Context,
	runID, destinationID string,
) (WebhookProjection, error) {
	if s == nil || ctx == nil || !webhookRunPattern.MatchString(runID) {
		return WebhookProjection{}, fail("INVALID_WEBHOOK_REQUEST")
	}
	endpoint, found := s.destinations[destinationID]
	if !found {
		return WebhookProjection{}, fail("WEBHOOK_DESTINATION_NOT_FOUND")
	}
	observer := webhookObserverPrefix + endpoint.id
	cursor, err := s.journal.ObserverCursor(ctx, runID, observer)
	if err != nil {
		return WebhookProjection{}, fail("JOURNAL_UNAVAILABLE")
	}
	window, err := s.journal.EventsAfter(
		ctx,
		runID,
		cursor,
		webhookProjectionLimit,
	)
	if err != nil {
		return WebhookProjection{}, fail("JOURNAL_UNAVAILABLE")
	}
	result := WebhookProjection{
		DestinationID: destinationID,
		ThroughOffset: cursor,
		HasMore:       window.HasMore,
	}
	if len(window.Events) == 0 {
		if window.HasMore || window.EventOffset != cursor {
			return WebhookProjection{}, fail("WEBHOOK_EVENT_GAP")
		}
		return result, nil
	}
	notifications := make(
		[]journal.NotificationDraft,
		0,
		len(window.Events),
	)
	for _, event := range window.Events {
		messageID := webhookMessageID(
			runID,
			endpoint.binding,
			event.Offset,
		)
		captainDecision, err := safeCaptainDecisionEvent(event)
		if err != nil {
			return WebhookProjection{}, fail("WEBHOOK_ENCODING_FAILED")
		}
		body, err := json.Marshal(webhookEventBody{
			SchemaVersion:      WebhookEventSchemaVersion,
			DestinationID:      destinationID,
			DestinationBinding: endpoint.binding,
			MessageID:          messageID,
			RunID:              runID,
			EventOffset:        event.Offset,
			EventKind:          safeWebhookEventKind(event.Kind),
			RecordedAt:         event.CreatedAt,
			CaptainDecision:    captainDecision,
		})
		if err != nil {
			return WebhookProjection{}, fail("WEBHOOK_ENCODING_FAILED")
		}
		notifications = append(notifications, journal.NotificationDraft{
			DestinationID:     destinationID,
			SourceEventOffset: event.Offset,
			MessageID:         messageID,
			Body:              body,
		})
	}
	if err := s.journal.AdvanceObserver(ctx, journal.ObserverAdvance{
		RunID:          runID,
		Observer:       observer,
		ExpectedOffset: cursor,
		ThroughOffset:  window.Through,
		Notifications:  notifications,
		At:             s.now().UTC(),
	}); err != nil {
		return WebhookProjection{}, fail("JOURNAL_UNAVAILABLE")
	}
	result.Projected = len(notifications)
	result.ThroughOffset = window.Through
	return result, nil
}

func safeCaptainDecisionEvent(event journal.EventFact) (*runtimepkg.CaptainDecisionEvent, error) {
	if event.Kind != "captain_plan_decided" {
		return nil, nil
	}
	var value runtimepkg.CaptainDecisionEvent
	decoder := json.NewDecoder(bytes.NewReader(event.SafeBody))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil {
		return nil, errors.New("invalid captain decision")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid captain decision")
	}
	canonical, err := json.Marshal(value)
	expectedSummary, expectedNext, mapped := runtimepkg.CaptainDecisionNotificationText(value.DecisionClass, value.Outcome)
	if err != nil || !bytes.Equal(canonical, event.SafeBody) || value.SchemaVersion != runtimepkg.CaptainDecisionEventVersion || value.RunID == "" || value.Project == "" || value.Release == "" || !mapped || value.ProposalReplayKey == "" || value.PlanDigest == "" || value.PlanRevision < 1 || value.TargetHead == "" || value.EnvelopeDigest == "" || value.EnvelopeEpoch < 1 || value.DecisionReplayKey == "" || value.Summary != expectedSummary || value.NextAction != expectedNext {
		return nil, errors.New("invalid captain decision")
	}
	return &value, nil
}

func webhookMessageID(
	runID, destinationBinding string,
	eventOffset int64,
) string {
	identity := runID + "\x00" + destinationBinding + "\x00" +
		strconv.FormatInt(eventOffset, 10)
	sum := sha256.Sum256([]byte(identity))
	return "webhook-" + hex.EncodeToString(sum[:])
}

func (s *WebhookService) Tick(
	ctx context.Context,
	runID string,
) (WebhookTick, error) {
	if s == nil || ctx == nil || !webhookRunPattern.MatchString(runID) {
		return WebhookTick{}, fail("INVALID_WEBHOOK_REQUEST")
	}
	destinations := make([]string, 0, len(s.destinations))
	for destinationID := range s.destinations {
		destinations = append(destinations, destinationID)
	}
	sort.Strings(destinations)
	type destinationResult struct {
		destination string
		projection  WebhookProjection
		delivery    WebhookDelivery
		err         error
	}
	results := make(chan destinationResult, len(destinations))
	var workers sync.WaitGroup
	for _, destinationID := range destinations {
		workers.Add(1)
		go func() {
			defer workers.Done()
			projection, projectionErr :=
				s.Project(ctx, runID, destinationID)
			delivery, deliveryErr :=
				s.DeliverOne(ctx, destinationID)
			results <- destinationResult{
				destination: destinationID,
				projection:  projection,
				delivery:    delivery,
				err:         errors.Join(projectionErr, deliveryErr),
			}
		}()
	}
	workers.Wait()
	close(results)
	items := make([]destinationResult, 0, len(destinations))
	for item := range results {
		items = append(items, item)
	}
	sort.Slice(items, func(left, right int) bool {
		return items[left].destination < items[right].destination
	})
	var result WebhookTick
	var tickErr error
	for _, item := range items {
		result.Projected += item.projection.Projected
		result.HasMore = result.HasMore || item.projection.HasMore
		if item.delivery.Found {
			result.Deliveries++
			result.HasMore = true
		}
		if tickErr == nil && item.err != nil {
			tickErr = item.err
		}
	}
	return result, tickErr
}

// Run performs bounded at-least-once delivery. Receivers must deduplicate
// retries by X-Sworn-Message-ID; attempts intentionally retain that ID.
func (s *WebhookService) Run(
	ctx context.Context,
	runID string,
	interval time.Duration,
) error {
	if s == nil || ctx == nil || !webhookRunPattern.MatchString(runID) ||
		interval < minWebhookInterval || interval > maxWebhookInterval {
		return fail("INVALID_WEBHOOK_RUN")
	}
	destinations := make([]string, 0, len(s.destinations))
	for destinationID := range s.destinations {
		destinations = append(destinations, destinationID)
	}
	sort.Strings(destinations)
	var workers sync.WaitGroup
	for _, destinationID := range destinations {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.runDestination(ctx, runID, destinationID, interval)
		}()
	}
	<-ctx.Done()
	workers.Wait()
	return nil
}

func (s *WebhookService) runDestination(
	ctx context.Context,
	runID, destinationID string,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Projection and delivery are independent durable steps. A damaged
		// observer cursor must not starve rows already committed to the outbox.
		_, _ = s.Project(ctx, runID, destinationID)
		_, _ = s.DeliverOne(ctx, destinationID)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *WebhookService) DeliverOne(
	ctx context.Context,
	destinationID string,
) (WebhookDelivery, error) {
	if s == nil || ctx == nil {
		return WebhookDelivery{}, fail("INVALID_WEBHOOK_REQUEST")
	}
	endpoint, found := s.destinations[destinationID]
	if !found {
		return WebhookDelivery{}, fail("WEBHOOK_DESTINATION_NOT_FOUND")
	}
	startedAt := s.now().UTC()
	claim, found, err := s.journal.ClaimNotification(
		ctx,
		destinationID,
		startedAt,
		webhookClaimLease,
	)
	if err != nil {
		return WebhookDelivery{}, fail("JOURNAL_UNAVAILABLE")
	}
	if !found {
		return WebhookDelivery{}, nil
	}
	disposition, errorCode := s.send(
		ctx,
		endpoint,
		claim.Notification,
		startedAt,
	)
	finishedAt := s.now().UTC()
	retryAt := time.Time{}
	if disposition == journal.NotificationRetry {
		retryAt = finishedAt.Add(
			webhookRetryDelay(claim.Notification.Attempts),
		)
	}
	finishContext, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		webhookFinishTimeout,
	)
	defer cancel()
	if err := s.journal.FinishNotification(
		finishContext,
		claim,
		disposition,
		retryAt,
		errorCode,
		finishedAt,
	); err != nil {
		return WebhookDelivery{}, fail("JOURNAL_UNAVAILABLE")
	}
	result := WebhookDelivery{
		Found:     true,
		RunID:     claim.Notification.RunID,
		MessageID: claim.Notification.MessageID,
		Attempts:  claim.Notification.Attempts,
		LastError: errorCode,
	}
	switch disposition {
	case journal.NotificationSucceeded:
		result.State = journal.NotificationDelivered
		result.DeliveredAt = finishedAt
	case journal.NotificationAbandon:
		result.State = journal.NotificationDead
	case journal.NotificationRetry:
		result.State = journal.NotificationPending
		if claim.Notification.Attempts >= journal.MaxNotificationAttempts {
			result.State = journal.NotificationDead
		}
	}
	return result, nil
}

func (s *WebhookService) send(
	ctx context.Context,
	endpoint *webhookEndpoint,
	item journal.Notification,
	sentAt time.Time,
) (journal.NotificationDisposition, string) {
	if code := validateWebhookEvent(
		endpoint,
		item,
	); code != "" {
		return journal.NotificationAbandon, code
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.url.String(),
		bytes.NewReader(item.Body),
	)
	if err != nil {
		return journal.NotificationAbandon, "WEBHOOK_REQUEST_INVALID"
	}
	canonical := webhookSignatureBytes(
		endpoint,
		item,
		sentAt.UTC(),
	)
	signature := hmac.New(sha256.New, endpoint.secret)
	_, _ = signature.Write(canonical)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "sworn-webhook/1")
	request.Header.Set("X-Sworn-Signature-Version", "v1")
	request.Header.Set("X-Sworn-Destination-ID", endpoint.id)
	request.Header.Set("X-Sworn-Message-ID", item.MessageID)
	request.Header.Set("X-Sworn-Attempt", strconv.FormatInt(item.Attempts, 10))
	request.Header.Set(
		"X-Sworn-Sent-At",
		sentAt.UTC().Format(time.RFC3339Nano),
	)
	request.Header.Set(
		"X-Sworn-Signature",
		"hmac-sha256="+hex.EncodeToString(signature.Sum(nil)),
	)
	response, err := endpoint.client.Do(request)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		code, permanent := classifyWebhookError(ctx, err)
		if permanent {
			return journal.NotificationAbandon, code
		}
		return journal.NotificationRetry, code
	}
	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		return journal.NotificationSucceeded, ""
	}
	if response.StatusCode == http.StatusRequestTimeout ||
		response.StatusCode == http.StatusTooEarly ||
		response.StatusCode == http.StatusTooManyRequests ||
		response.StatusCode >= 500 {
		return journal.NotificationRetry, "WEBHOOK_HTTP_RETRYABLE"
	}
	return journal.NotificationAbandon, "WEBHOOK_HTTP_REJECTED"
}

func validateWebhookEvent(
	endpoint *webhookEndpoint,
	item journal.Notification,
) string {
	var value webhookEventBody
	decoder := json.NewDecoder(bytes.NewReader(item.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return "WEBHOOK_PAYLOAD_INVALID"
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "WEBHOOK_PAYLOAD_INVALID"
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, item.Body) {
		return "WEBHOOK_PAYLOAD_INVALID"
	}
	if value.DestinationID != endpoint.id ||
		value.DestinationBinding != endpoint.binding {
		return "WEBHOOK_CONFIG_CHANGED"
	}
	if value.SchemaVersion != WebhookEventSchemaVersion ||
		value.MessageID != item.MessageID ||
		value.RunID != item.RunID ||
		value.EventOffset != item.SourceEventOffset ||
		value.EventOffset < 1 || !validSafeWebhookEventKind(value.EventKind) ||
		value.RecordedAt.IsZero() ||
		webhookMessageID(
			value.RunID,
			value.DestinationBinding,
			value.EventOffset,
		) != value.MessageID {
		return "WEBHOOK_PAYLOAD_INVALID"
	}
	if value.CaptainDecision != nil {
		body, marshalErr := json.Marshal(value.CaptainDecision)
		validated, validateErr := safeCaptainDecisionEvent(journal.EventFact{Kind: "captain_plan_decided", SafeBody: body})
		if marshalErr != nil || validateErr != nil || validated.RunID != value.RunID || value.EventKind != webhookRunUpdated {
			return "WEBHOOK_PAYLOAD_INVALID"
		}
	}
	return ""
}

func safeWebhookEventKind(kind string) string {
	switch {
	case kind == "control_accepted" ||
		kind == "owner_acquired" ||
		kind == "owner_released":
		return webhookControlUpdated
	case kind == journal.AttentionOpenedEvent ||
		kind == journal.AttentionAnsweredEvent ||
		kind == journal.AttentionResolvedEvent ||
		kind == journal.RecoveryStepReservedEvent ||
		kind == journal.RecoveryResumeWorkerEvent ||
		kind == journal.RecoveryAskCaptainEvent ||
		kind == journal.RecoveryRetryOperationalEvent ||
		kind == journal.RecoveryParkedEvent ||
		kind == "turn_recovery.outcome.recovered" ||
		kind == "turn_recovery.outcome.human_escalation" ||
		kind == "turn_recovery.outcome.false_acceptance":
		return webhookRecoveryUpdated
	case strings.Contains(kind, "uncertain") ||
		strings.Contains(kind, "reconciled") ||
		strings.Contains(kind, "recovered") ||
		strings.Contains(kind, "rolled_back"):
		return webhookRecoveryUpdated
	case strings.HasPrefix(kind, "dispatch_"):
		return webhookAttemptUpdated
	default:
		return webhookRunUpdated
	}
}

func validSafeWebhookEventKind(kind string) bool {
	switch kind {
	case webhookRunUpdated, webhookControlUpdated, webhookAttemptUpdated,
		webhookRecoveryUpdated:
		return true
	default:
		return false
	}
}

func webhookSignatureBytes(
	endpoint *webhookEndpoint,
	item journal.Notification,
	sentAt time.Time,
) []byte {
	bodyDigest := sha256.Sum256(item.Body)
	body, _ := json.Marshal(webhookSignatureInput{
		Version:            "v1",
		Method:             http.MethodPost,
		Authority:          endpoint.url.Host,
		Path:               endpoint.url.EscapedPath(),
		DestinationID:      endpoint.id,
		DestinationBinding: endpoint.binding,
		MessageID:          item.MessageID,
		Attempt:            item.Attempts,
		SentAt:             sentAt.UTC().Format(time.RFC3339Nano),
		BodyDigest:         "sha256:" + hex.EncodeToString(bodyDigest[:]),
	})
	return body
}

func webhookRetryDelay(attempt int64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > journal.MaxNotificationAttempts {
		attempt = journal.MaxNotificationAttempts
	}
	return webhookRetryBase << (attempt - 1)
}

func classifyWebhookError(
	ctx context.Context,
	err error,
) (string, bool) {
	if errors.Is(err, errWebhookRedirect) {
		return "WEBHOOK_REDIRECT", true
	}
	var networkError *webhookNetworkError
	if errors.As(err, &networkError) {
		switch networkError.code {
		case "WEBHOOK_DNS_REJECTED", "WEBHOOK_AUTHORITY_CHANGED":
			return networkError.code, true
		default:
			return networkError.code, false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "WEBHOOK_TIMEOUT", false
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(ctx.Err(), context.Canceled) {
		return "WEBHOOK_CANCELLED", false
	}
	return "WEBHOOK_TRANSPORT", false
}

type safeWebhookDialer struct {
	host     string
	port     string
	resolver webhookResolver
	dialer   webhookDialer
}

func (d *safeWebhookDialer) DialContext(
	ctx context.Context,
	network, address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || !strings.HasPrefix(network, "tcp") ||
		!strings.EqualFold(host, d.host) || port != d.port {
		return nil, &webhookNetworkError{code: "WEBHOOK_AUTHORITY_CHANGED"}
	}
	resolved, err := d.resolver.LookupIPAddr(ctx, d.host)
	if err != nil || len(resolved) == 0 ||
		len(resolved) > maxWebhookAddresses {
		return nil, &webhookNetworkError{code: "WEBHOOK_DNS_UNAVAILABLE"}
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	for _, candidate := range resolved {
		value, ok := netip.AddrFromSlice(candidate.IP)
		if !ok {
			return nil, &webhookNetworkError{code: "WEBHOOK_DNS_REJECTED"}
		}
		value = value.Unmap()
		if !publicWebhookIP(value) {
			return nil, &webhookNetworkError{code: "WEBHOOK_DNS_REJECTED"}
		}
		addresses = append(addresses, value)
	}
	sort.Slice(addresses, func(left, right int) bool {
		return addresses[left].Less(addresses[right])
	})
	for _, candidate := range addresses {
		connection, err := d.dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(candidate.String(), port),
		)
		if err == nil {
			return connection, nil
		}
	}
	return nil, &webhookNetworkError{code: "WEBHOOK_TRANSPORT"}
}

func publicWebhookIP(value netip.Addr) bool {
	if !value.IsValid() || !value.IsGlobalUnicast() {
		return false
	}
	for _, denied := range webhookDeniedRanges {
		if denied.Contains(value) {
			return false
		}
	}
	return true
}
