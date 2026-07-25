package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"sync"
)

const (
	SubmissionControlVersion      = "sworn.submission-control/v1"
	SubmissionProtocolID          = "sworn.submission-control/v1"
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

func serveSubmissionEndpoint(stream io.ReadWriter, server *SubmissionServer) error {
	if closer, ok := stream.(io.Closer); ok {
		defer closer.Close()
	}
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
		if request.InvocationID != server.permission.descriptor.InvocationID {
			return fail("SUBMISSION_BINDING_MISMATCH")
		}
		response := endpointResponse{
			SchemaVersion: SubmissionControlVersion,
			Type:          request.Type,
			Code:          "ok",
		}
		switch request.Type {
		case "describe":
			descriptor, err := server.Describe()
			if err != nil {
				return err
			}
			response.Descriptor = &descriptor
		case "submit":
			body, err := base64.StdEncoding.Strict().DecodeString(request.Submission)
			if err != nil || base64.StdEncoding.EncodeToString(body) != request.Submission {
				return fail("INVALID_SUBMISSION")
			}
			seal, sealBytes, submitErr := server.Submit(body)
			response.Seal = &seal
			response.SealBytes = base64.StdEncoding.EncodeToString(sealBytes)
			if submitErr != nil {
				response.Code = "rejected"
			}
		default:
			return fail("INVALID_CONTROL_REQUEST")
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
	value, err := decodeStrict(body, MaxFrameBytes)
	if err != nil {
		return endpointRequest{}, err
	}
	root, err := closedObject(value,
		[]string{"schema_version", "type", "invocation_id"}, []string{"submission"})
	if err != nil {
		return endpointRequest{}, err
	}
	var request endpointRequest
	if request.SchemaVersion, err = requiredString(root, "schema_version"); err != nil {
		return endpointRequest{}, err
	}
	if request.Type, err = requiredString(root, "type"); err != nil {
		return endpointRequest{}, err
	}
	if request.InvocationID, err = requiredString(root, "invocation_id"); err != nil {
		return endpointRequest{}, err
	}
	if _, present := root["submission"]; present {
		if request.Submission, err = requiredStringAllowEmpty(root, "submission"); err != nil {
			return endpointRequest{}, err
		}
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
	if os.Getenv(SubmissionProtocolEnvironment) != SubmissionProtocolID ||
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
		return *response.Seal, sealBytes, fail("SUBMISSION_REJECTED")
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
	value, err := decodeStrict(responseBody, MaxFrameBytes)
	if err != nil {
		return endpointResponse{}, err
	}
	root, err := closedObject(value,
		[]string{"schema_version", "type", "code", "descriptor", "seal", "seal_bytes"}, nil)
	if err != nil {
		return endpointResponse{}, err
	}
	_ = root
	var response endpointResponse
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return endpointResponse{}, fail("INVALID_CONTROL_RESPONSE")
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
