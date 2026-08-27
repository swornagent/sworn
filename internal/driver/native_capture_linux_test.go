//go:build linux

package driver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeProviderCaptureConsumesMalformedFirstRequest(t *testing.T) {
	const model = "sworn-capture-model"
	malformed := codexFirstProviderRequestFixture(t, model, ReadWrite)
	malformed["parallel_tool_calls"] = true
	exactBody := marshalNativeCaptureRequest(
		t,
		codexFirstProviderRequestFixture(t, model, ReadWrite),
	)
	for _, test := range []struct {
		name        string
		contentType string
		body        []byte
	}{
		{
			name:        "tool surface",
			contentType: "application/json",
			body:        marshalNativeCaptureRequest(t, malformed),
		},
		{
			name:        "body",
			contentType: "application/json",
			body:        []byte(`{`),
		},
		{
			name:        "content type",
			contentType: "text/plain",
			body:        exactBody,
		},
		{
			name:        "content type suffix",
			contentType: "application/jsonp",
			body:        exactBody,
		},
		{
			name:        "content type malformed parameter",
			contentType: "application/json; charset",
			body:        exactBody,
		},
		{
			name:        "content type malformed quoted parameter",
			contentType: `application/json; charset="utf-8`,
			body:        exactBody,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture, err := newNativeProviderCapture(
				ProfileCodex,
				model,
				ReadWrite,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			if status := sendNativeCaptureRequestWithContentType(
				t,
				capture,
				http.MethodPost,
				capture.BaseURL()+"/responses",
				true,
				test.contentType,
				test.body,
			); status != http.StatusBadRequest {
				t.Fatalf("malformed first status = %d", status)
			}
			if status := sendNativeCaptureRequest(
				t,
				capture,
				http.MethodPost,
				capture.BaseURL()+"/responses",
				true,
				exactBody,
			); status != http.StatusConflict {
				t.Fatalf("exact retry status = %d", status)
			}
			assertNativeCaptureEmpty(t, capture)
		})
	}
	t.Run("read failure", func(t *testing.T) {
		capture, err := newNativeProviderCapture(
			ProfileCodex,
			model,
			ReadWrite,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer capture.Close()
		request, err := http.NewRequest(
			http.MethodPost,
			capture.BaseURL()+"/responses",
			nativeCaptureReadFailure{},
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		token := capture.bearer()
		request.Header.Set("Authorization", "Bearer "+string(token))
		clearBytes(token)
		response := httptest.NewRecorder()
		capture.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("read failure status = %d", response.Code)
		}
		if status := sendNativeCaptureRequest(
			t,
			capture,
			http.MethodPost,
			capture.BaseURL()+"/responses",
			true,
			exactBody,
		); status != http.StatusConflict {
			t.Fatalf("exact retry status = %d", status)
		}
		assertNativeCaptureEmpty(t, capture)
	})
}

func TestNativeProviderCaptureRecordsOnlyValidFirstRequest(t *testing.T) {
	const model = "sworn-capture-model"
	capture, err := newNativeProviderCapture(
		ProfileCodex,
		model,
		ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()

	if status := sendNativeCaptureRequestWithContentType(
		t,
		capture,
		http.MethodPost,
		capture.BaseURL()+"/responses",
		true,
		`Application/JSON; Charset="UTF-8"`,
		marshalNativeCaptureRequest(
			t,
			codexFirstProviderRequestFixture(t, model, ReadWrite),
		),
	); status != http.StatusServiceUnavailable {
		t.Fatalf("valid first status = %d", status)
	}
	select {
	case evidence := <-capture.Captured():
		if evidence.RequestDigest == "" ||
			evidence.ToolDigest != nativeToolSurfaceDigest(ReadWrite) {
			t.Fatalf("evidence = %#v", evidence)
		}
	default:
		t.Fatal("valid first request did not record evidence")
	}
	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		capture.BaseURL()+"/responses",
		true,
		marshalNativeCaptureRequest(
			t,
			codexFirstProviderRequestFixture(t, model, ReadWrite),
		),
	); status != http.StatusConflict {
		t.Fatalf("valid retry status = %d", status)
	}
	assertNativeCaptureEmpty(t, capture)
}

func TestNativeProviderCaptureIgnoresUnadmittedRequests(t *testing.T) {
	const model = "sworn-capture-model"
	capture, err := newNativeProviderCapture(
		ProfileCodex,
		model,
		ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	body := marshalNativeCaptureRequest(
		t,
		codexFirstProviderRequestFixture(t, model, ReadWrite),
	)

	for _, request := range []struct {
		name       string
		method     string
		url        string
		authorized bool
	}{
		{
			name: "preflight", method: http.MethodOptions,
			url: capture.BaseURL() + "/responses", authorized: true,
		},
		{
			name: "unauthorized", method: http.MethodPost,
			url: capture.BaseURL() + "/responses",
		},
		{
			name: "wrong endpoint", method: http.MethodPost,
			url: capture.BaseURL() + "/other", authorized: true,
		},
	} {
		t.Run(request.name, func(t *testing.T) {
			if status := sendNativeCaptureRequest(
				t,
				capture,
				request.method,
				request.url,
				request.authorized,
				body,
			); status != http.StatusBadRequest {
				t.Fatalf("status = %d", status)
			}
		})
	}
	assertNativeCaptureEmpty(t, capture)
	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		capture.BaseURL()+"/responses",
		true,
		body,
	); status != http.StatusServiceUnavailable {
		t.Fatalf("valid admitted status = %d", status)
	}
	select {
	case <-capture.Captured():
	default:
		t.Fatal("valid admitted request did not record evidence")
	}
}

func TestNativeProviderCaptureAdmitsClaudeToolLessRequest(t *testing.T) {
	const model = "sworn-capture-model"
	capture, err := newNativeProviderCapture(ProfileClaude, model, ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	url := capture.BaseURL() + "/v1/messages?beta=true"

	toolLess := map[string]any{"model": "claude-compaction-model"}
	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		url,
		true,
		marshalNativeCaptureRequest(t, toolLess),
	); status != http.StatusBadRequest {
		t.Fatalf("tool-less status = %d", status)
	}
	assertNativeCaptureEmpty(t, capture)

	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		url,
		true,
		marshalNativeCaptureRequest(
			t,
			claudeFirstProviderRequestFixture(t, model, ReadWrite),
		),
	); status != http.StatusServiceUnavailable {
		t.Fatalf("tool-bearing status = %d", status)
	}
	select {
	case evidence := <-capture.Captured():
		if evidence.RequestDigest == "" ||
			evidence.ToolDigest != nativeToolSurfaceDigest(ReadWrite) {
			t.Fatalf("evidence = %#v", evidence)
		}
	default:
		t.Fatal("tool-bearing request did not record evidence")
	}
}

func TestNativeProviderCaptureConsumesMutatedClaudeToolSurface(t *testing.T) {
	const model = "sworn-capture-model"
	capture, err := newNativeProviderCapture(ProfileClaude, model, ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	url := capture.BaseURL() + "/v1/messages?beta=true"

	mutated := claudeFirstProviderRequestFixture(t, model, ReadWrite)
	mutated["tools"].([]any)[0].(map[string]any)["description"] = "changed"
	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		url,
		true,
		marshalNativeCaptureRequest(t, mutated),
	); status != http.StatusBadRequest {
		t.Fatalf("mutated status = %d", status)
	}
	if status := sendNativeCaptureRequest(
		t,
		capture,
		http.MethodPost,
		url,
		true,
		marshalNativeCaptureRequest(
			t,
			claudeFirstProviderRequestFixture(t, model, ReadWrite),
		),
	); status != http.StatusConflict {
		t.Fatalf("retry status = %d", status)
	}
	assertNativeCaptureEmpty(t, capture)
}

func marshalNativeCaptureRequest(t *testing.T, request map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func sendNativeCaptureRequest(
	t *testing.T,
	capture *nativeProviderCapture,
	method string,
	url string,
	authorized bool,
	body []byte,
) int {
	return sendNativeCaptureRequestWithContentType(
		t,
		capture,
		method,
		url,
		authorized,
		"application/json",
		body,
	)
}

func sendNativeCaptureRequestWithContentType(
	t *testing.T,
	capture *nativeProviderCapture,
	method string,
	url string,
	authorized bool,
	contentType string,
	body []byte,
) int {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", contentType)
	if authorized {
		token := capture.bearer()
		request.Header.Set("Authorization", "Bearer "+string(token))
		clearBytes(token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return response.StatusCode
}

func assertNativeCaptureEmpty(
	t *testing.T,
	capture *nativeProviderCapture,
) {
	t.Helper()
	select {
	case evidence := <-capture.Captured():
		t.Fatalf("unexpected capture evidence = %#v", evidence)
	default:
	}
}

type nativeCaptureReadFailure struct{}

func (nativeCaptureReadFailure) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
