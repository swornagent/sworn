package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"sync"
)

const (
	SubmissionControlVersion      = "sworn.submission-control/v1"
	SubmissionFDEnvironment       = "SWORN_SUBMISSION_FD"
	SubmissionProtocolEnvironment = "SWORN_SUBMISSION_PROTOCOL"
)

type endpointRequest struct {
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	InvocationID  string `json:"invocation_id"`
	Submission    string `json:"submission,omitempty"`
}
type endpointResponse struct {
	SchemaVersion string                `json:"schema_version"`
	Type          string                `json:"type"`
	Code          string                `json:"code"`
	Descriptor    *PermissionDescriptor `json:"descriptor"`
	Seal          *Seal                 `json:"seal"`
	SealBytes     string                `json:"seal_bytes"`
}

const (
	fatalPostcheck = iota + 1
	fatalTransport
	fatalProtocol
	fatalOverflow
	fatalTimeout
	fatalCancellation
)

type terminalArbiter struct {
	mu                                      sync.Mutex
	cond                                    *sync.Cond
	server                                  *submissionServer
	binding                                 ResultBinding
	outputLimit, outputMaximum, outputTotal int64
	output                                  []byte
	result                                  *Result
	usage                                   UsageReceipt
	attempted, accepted                     bool
	provisional, acknowledged               bool
	engineStopped, engineSignalled          bool
	published, finished                     bool
	submissionBody                          []byte
	seal                                    Seal
	sealBytes                               []byte
	fatal                                   error
	fatalCode                               string
	fatalPriority                           int
	sequence                                uint64
	events                                  []TerminalEvent
	stop                                    func() bool
}

func newTerminalArbiter(invocation Invocation, server *submissionServer) *terminalArbiter {
	maximum := invocation.Request.Limits.OutputBytes + MaxResultEnvelopeBytes
	if maximum > MaxStdoutBytes {
		maximum = MaxStdoutBytes
	}
	arbiter := &terminalArbiter{
		server:        server,
		outputLimit:   invocation.Request.Limits.OutputBytes,
		outputMaximum: maximum,
		binding: ResultBinding{
			InvocationID:  invocation.Request.InvocationID,
			DriverID:      invocation.Selected.Provider.DriverID,
			DriverVersion: invocation.Selected.Provider.DriverVersion,
			Model:         &invocation.Selected.Model,
			BindModel:     true,
		},
	}
	arbiter.cond = sync.NewCond(&arbiter.mu)
	return arbiter
}
func (arbiter *terminalArbiter) eventLocked(kind string) {
	arbiter.sequence++
	arbiter.events = append(arbiter.events, TerminalEvent{
		Sequence: arbiter.sequence,
		Kind:     kind,
	})
}
func (arbiter *terminalArbiter) failLocked(code string, err error, priority int) {
	if priority > arbiter.fatalPriority {
		arbiter.fatal = err
		arbiter.fatalCode = code
		arbiter.fatalPriority = priority
	}
	arbiter.eventLocked("fatal:" + code)
	arbiter.provisional = false
	arbiter.cond.Broadcast()
}
func (arbiter *terminalArbiter) fail(code string, err error, priority int) {
	arbiter.mu.Lock()
	arbiter.failLocked(code, err, priority)
	stop := arbiter.stop
	arbiter.mu.Unlock()
	if stop != nil {
		stop()
	}
}
func (arbiter *terminalArbiter) cancel(code string, err error, priority int) {
	arbiter.mu.Lock()
	finished := arbiter.finished
	if !finished {
		arbiter.failLocked(code, err, priority)
	}
	stop := arbiter.stop
	arbiter.mu.Unlock()
	if !finished && stop != nil {
		stop()
	}
}
func (arbiter *terminalArbiter) Write(body []byte) (int, error) {
	arbiter.mu.Lock()
	arbiter.outputTotal += int64(len(body))
	switch {
	case arbiter.fatal != nil:
	case arbiter.outputTotal > arbiter.outputMaximum:
		arbiter.failLocked("stdout_overflow", fail("OUTPUT_OVERFLOW"), fatalOverflow)
	case arbiter.result != nil:
		arbiter.failLocked("post_result_stdout", fail("PROTOCOL_FAILURE"), fatalProtocol)
	default:
		arbiter.output = append(arbiter.output, body...)
		if end := bytes.IndexByte(arbiter.output, '\n'); end >= 0 {
			if end != len(arbiter.output)-1 {
				arbiter.failLocked("extra_stdout", fail("PROTOCOL_FAILURE"), fatalProtocol)
				break
			}
			result, err := DecodeResult(arbiter.output, arbiter.binding)
			if err != nil {
				arbiter.failLocked("invalid_driver_result", err, fatalProtocol)
				break
			}
			if int64(len([]byte(result.Text))) > arbiter.outputLimit {
				arbiter.failLocked("result_limit_exceeded", fail("RESOURCE_LIMIT"), fatalOverflow)
				break
			}
			if result.TransportStatus != Completed {
				arbiter.failLocked("driver_transport_failed", fail("TRANSPORT_FAILURE"), fatalTransport)
				break
			}
			usage, err := NormalizeUsage(result.Usage, nil)
			if err != nil {
				arbiter.failLocked("invalid_usage", err, fatalProtocol)
				break
			}
			arbiter.result = &result
			arbiter.usage = usage
			arbiter.eventLocked("result_completed")
			arbiter.cond.Broadcast()
		}
	}
	stop := arbiter.stop
	failed := arbiter.fatal != nil
	arbiter.mu.Unlock()
	if failed && stop != nil {
		stop()
	}
	return len(body), nil
}
func (arbiter *terminalArbiter) submit(body []byte) (Seal, []byte, error) {
	arbiter.mu.Lock()
	defer arbiter.mu.Unlock()
	if arbiter.attempted {
		arbiter.failLocked("late_submission", fail("SUBMISSION_CONFLICT"), fatalProtocol)
		return arbiter.seal, append([]byte(nil), arbiter.sealBytes...), arbiter.fatal
	}
	arbiter.attempted = true
	seal, sealBytes, submitErr := arbiter.server.Submit(body)
	arbiter.submissionBody = append([]byte(nil), body...)
	arbiter.seal = seal
	arbiter.sealBytes = append([]byte(nil), sealBytes...)
	arbiter.accepted = seal.Accepted
	if seal.Accepted {
		arbiter.eventLocked("submit_accepted_pending")
	} else {
		arbiter.eventLocked("submit_rejected_pending")
	}
	for arbiter.result == nil && arbiter.fatal == nil {
		arbiter.cond.Wait()
	}
	if arbiter.fatal != nil {
		return Seal{}, nil, arbiter.fatal
	}
	return seal, append([]byte(nil), sealBytes...), submitErr
}
func (arbiter *terminalArbiter) writeAcknowledgement(
	stream io.Writer,
	response endpointResponse,
) error {
	body, err := json.Marshal(response)
	if err != nil {
		arbiter.fail("submission_protocol_failed", fail("INVALID_JSON"), fatalProtocol)
		return fail("INVALID_JSON")
	}
	arbiter.mu.Lock()
	if arbiter.fatal != nil {
		err := arbiter.fatal
		arbiter.mu.Unlock()
		return err
	}
	arbiter.provisional = arbiter.accepted
	if err := WriteFrame(stream, append(body, '\n')); err != nil {
		arbiter.provisional = false
		arbiter.failLocked("submission_protocol_failed", err, fatalProtocol)
		stop := arbiter.stop
		arbiter.mu.Unlock()
		if stop != nil {
			stop()
		}
		return err
	}
	arbiter.acknowledged = true
	arbiter.eventLocked("submit_acknowledged")
	arbiter.engineStopped = true
	arbiter.eventLocked("engine_stop_after_submit")
	if arbiter.stop != nil {
		arbiter.engineSignalled = arbiter.stop()
	}
	arbiter.mu.Unlock()
	return nil
}
func (arbiter *terminalArbiter) processDone(waitErr error, engineExit bool) {
	arbiter.mu.Lock()
	defer arbiter.mu.Unlock()
	arbiter.eventLocked("process_waited")
	if arbiter.result == nil && arbiter.fatal == nil {
		arbiter.failLocked("invalid_driver_result", fail("INVALID_RESULT"), fatalProtocol)
	}
	if waitErr != nil && arbiter.fatal == nil &&
		(!arbiter.engineStopped || !arbiter.engineSignalled || !engineExit) {
		arbiter.failLocked("process_failed", fail("PROCESS_FAILED"), fatalTransport)
	}
	if arbiter.attempted && !arbiter.engineStopped && arbiter.fatal == nil {
		arbiter.failLocked("submit_without_engine_stop", fail("PROTOCOL_FAILURE"), fatalProtocol)
	}
}
func (arbiter *terminalArbiter) mark(kind string) {
	arbiter.mu.Lock()
	arbiter.eventLocked(kind)
	arbiter.mu.Unlock()
}
func (arbiter *terminalArbiter) terminalError() error {
	arbiter.mu.Lock()
	defer arbiter.mu.Unlock()
	return arbiter.fatal
}
func (arbiter *terminalArbiter) publish(contextErr error) {
	arbiter.mu.Lock()
	defer arbiter.mu.Unlock()
	if contextErr != nil {
		code := "invocation_cancelled"
		err := fail("INVOCATION_CANCELLED")
		priority := fatalCancellation
		if contextErr == context.DeadlineExceeded {
			code, err, priority = "invocation_timeout", fail("INVOCATION_TIMEOUT"), fatalTimeout
		}
		arbiter.failLocked(code, err, priority)
		return
	}
	if arbiter.fatal != nil {
		return
	}
	if arbiter.result == nil {
		arbiter.failLocked("invalid_driver_result", fail("INVALID_RESULT"), fatalProtocol)
		return
	}
	if arbiter.accepted &&
		(!arbiter.provisional || !arbiter.acknowledged || !arbiter.engineStopped) {
		arbiter.failLocked("publication_gate_failed", fail("PROTOCOL_FAILURE"), fatalProtocol)
		return
	}
	if arbiter.accepted &&
		(arbiter.seal.InvocationID != arbiter.binding.InvocationID ||
			arbiter.seal.SubmissionDigest != Digest(arbiter.submissionBody)) {
		arbiter.failLocked("submission_binding_failed", fail("SUBMISSION_BINDING_MISMATCH"), fatalProtocol)
		return
	}
	arbiter.published = arbiter.accepted
	arbiter.finished = true
	if arbiter.accepted {
		arbiter.eventLocked("published")
	} else {
		arbiter.eventLocked("completed_without_handoff")
	}
}
func (arbiter *terminalArbiter) observation() (Observation, error) {
	arbiter.mu.Lock()
	defer arbiter.mu.Unlock()
	events := append([]TerminalEvent(nil), arbiter.events...)
	diagnostic := Diagnostic{Code: "none"}
	if arbiter.fatal != nil {
		diagnostic.Code = arbiter.fatalCode
		return Observation{Diagnostic: diagnostic, Events: events}, arbiter.fatal
	}
	result := *arbiter.result
	observation := Observation{
		TransportStatus: result.TransportStatus,
		DurationMillis:  result.DurationMillis,
		TextBytes:       int64(len([]byte(result.Text))),
		TextDigest:      Digest([]byte(result.Text)),
		Usage:           arbiter.usage,
		Diagnostic:      diagnostic,
		Events:          events,
	}
	if !arbiter.accepted {
		if arbiter.attempted {
			observation.Diagnostic.Code = "submission_rejected"
		} else {
			observation.Diagnostic.Code = "submission_absent"
		}
		return observation, nil
	}
	if !arbiter.published {
		observation.Diagnostic.Code = "publication_gate_failed"
		return observation, fail("PROTOCOL_FAILURE")
	}
	observation.Handoff = &SealedHandoff{
		SubmissionBytes:  append([]byte(nil), arbiter.submissionBody...),
		SubmissionDigest: Digest(arbiter.submissionBody),
		SealBytes:        append([]byte(nil), arbiter.sealBytes...),
		SealDigest:       Digest(arbiter.sealBytes),
	}
	return observation, nil
}
func serveSubmissionEndpoint(stream io.ReadWriter, arbiter *terminalArbiter) (resultErr error) {
	if closer, ok := stream.(io.Closer); ok {
		defer closer.Close()
	}
	defer func() {
		if resultErr != nil && arbiter.terminalError() == nil {
			arbiter.fail("submission_protocol_failed", fail("SUBMISSION_PROTOCOL_FAILED"), fatalProtocol)
		}
	}()
	for {
		payload, err := ReadFrame(stream)
		if err != nil {
			if IsCode(err, "ENDPOINT_CLOSED") {
				return nil
			}
			return err
		}
		request, err := decodeEndpointRequest(payload)
		if err != nil {
			return err
		}
		if request.InvocationID != arbiter.server.permission.descriptor.InvocationID {
			err := fail("SUBMISSION_BINDING_MISMATCH")
			arbiter.fail("submission_protocol_failed", err, fatalProtocol)
			return err
		}
		response := endpointResponse{
			SchemaVersion: SubmissionControlVersion,
			Type:          request.Type,
			Code:          "ok",
		}
		switch request.Type {
		case "describe":
			descriptor, err := arbiter.server.permission.Describe()
			if err != nil {
				return err
			}
			response.Descriptor = &descriptor
		case "submit":
			body, err := base64.StdEncoding.Strict().DecodeString(request.Submission)
			if err != nil || base64.StdEncoding.EncodeToString(body) != request.Submission {
				body = nil
			}
			seal, sealBytes, submitErr := arbiter.submit(body)
			if fatalErr := arbiter.terminalError(); fatalErr != nil {
				return fatalErr
			}
			response.Seal = &seal
			response.SealBytes = base64.StdEncoding.EncodeToString(sealBytes)
			if submitErr != nil {
				response.Code = "SUBMISSION_REJECTED"
				if IsCode(submitErr, "SUBMISSION_CONFLICT") {
					response.Code = "SUBMISSION_CONFLICT"
				}
			}
		default:
			err := fail("INVALID_CONTROL_REQUEST")
			return err
		}
		if request.Type == "submit" {
			return arbiter.writeAcknowledgement(stream, response)
		}
		body, err := json.Marshal(response)
		if err != nil {
			return fail("INVALID_JSON")
		}
		if err := WriteFrame(stream, append(body, '\n')); err != nil {
			return err
		}
	}
}
func decodeEndpointRequest(body []byte) (endpointRequest, error) {
	var request endpointRequest
	root, err := decodeTyped(
		body,
		MaxFrameBytes,
		[]string{"schema_version", "type", "invocation_id"},
		[]string{"submission"},
		&request,
	)
	if err != nil {
		return endpointRequest{}, err
	}
	if _, present := root["submission"]; present {
		if request.Type != "submit" {
			return endpointRequest{}, fail("UNKNOWN_FIELD")
		}
	} else if request.Type == "submit" {
		return endpointRequest{}, fail("MISSING_FIELD")
	}
	if request.SchemaVersion != SubmissionControlVersion {
		return endpointRequest{}, fail("INVALID_VERSION")
	}
	canonical, err := json.Marshal(request)
	if err != nil {
		return endpointRequest{}, fail("INVALID_JSON")
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(canonical, body) {
		return endpointRequest{}, fail("NONCANONICAL_JSON")
	}
	return request, nil
}

type EndpointClient struct {
	mu           sync.Mutex
	stream       io.ReadWriter
	invocationID string
}

func NewEndpointClient(stream io.ReadWriter, invocationID string) (*EndpointClient, error) {
	if stream == nil {
		return nil, fail("INVALID_ENDPOINT")
	}
	if err := validateIdentity(invocationID); err != nil {
		return nil, err
	}
	return &EndpointClient{stream: stream, invocationID: invocationID}, nil
}
func NewEndpointClientFromEnvironment(invocationID string) (*EndpointClient, *os.File, error) {
	if os.Getenv(SubmissionProtocolEnvironment) != SubmissionControlVersion ||
		os.Getenv(SubmissionFDEnvironment) != "3" {
		return nil, nil, fail("INVALID_ENDPOINT")
	}
	file := os.NewFile(3, "sworn-submission")
	if file == nil {
		return nil, nil, fail("INVALID_ENDPOINT")
	}
	client, err := NewEndpointClient(file, invocationID)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return client, file, nil
}
func (client *EndpointClient) Describe() (PermissionDescriptor, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	response, err := client.exchange(endpointRequest{
		SchemaVersion: SubmissionControlVersion,
		Type:          "describe",
		InvocationID:  client.invocationID,
	})
	if err != nil {
		return PermissionDescriptor{}, err
	}
	if response.Type != "describe" || response.Code != "ok" ||
		response.Descriptor == nil || response.Seal != nil || response.SealBytes != "" {
		return PermissionDescriptor{}, fail("INVALID_CONTROL_RESPONSE")
	}
	return *response.Descriptor, nil
}
func (client *EndpointClient) Submit(body []byte) (Seal, []byte, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	response, err := client.exchange(endpointRequest{
		SchemaVersion: SubmissionControlVersion,
		Type:          "submit",
		InvocationID:  client.invocationID,
		Submission:    base64.StdEncoding.EncodeToString(body),
	})
	if err != nil {
		return Seal{}, nil, err
	}
	if response.Type != "submit" || response.Descriptor != nil || response.Seal == nil {
		return Seal{}, nil, fail("INVALID_CONTROL_RESPONSE")
	}
	sealBytes, err := base64.StdEncoding.Strict().DecodeString(response.SealBytes)
	if err != nil || base64.StdEncoding.EncodeToString(sealBytes) != response.SealBytes {
		return Seal{}, nil, fail("INVALID_CONTROL_RESPONSE")
	}
	if response.Code != "ok" {
		switch response.Code {
		case "SUBMISSION_REJECTED", "SUBMISSION_CONFLICT":
			return *response.Seal, sealBytes, fail(response.Code)
		default:
			return Seal{}, nil, fail("INVALID_CONTROL_RESPONSE")
		}
	}
	return *response.Seal, sealBytes, nil
}
func (client *EndpointClient) exchange(request endpointRequest) (endpointResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return endpointResponse{}, fail("INVALID_JSON")
	}
	if err := WriteFrame(client.stream, append(body, '\n')); err != nil {
		return endpointResponse{}, err
	}
	responseBody, err := ReadFrame(client.stream)
	if err != nil {
		return endpointResponse{}, err
	}
	var response endpointResponse
	if _, err := decodeTyped(
		responseBody,
		MaxFrameBytes,
		[]string{"schema_version", "type", "code", "descriptor", "seal", "seal_bytes"},
		nil,
		&response,
	); err != nil {
		return endpointResponse{}, err
	}
	canonical, err := json.Marshal(response)
	if err != nil {
		return endpointResponse{}, fail("INVALID_JSON")
	}
	canonical = append(canonical, '\n')
	if response.SchemaVersion != SubmissionControlVersion ||
		!bytes.Equal(canonical, responseBody) {
		return endpointResponse{}, fail("INVALID_CONTROL_RESPONSE")
	}
	return response, nil
}
