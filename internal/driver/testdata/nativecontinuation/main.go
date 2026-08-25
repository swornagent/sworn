package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sessionID = "11111111-1111-4111-8111-111111111111"

type promptEnvelope struct {
	InvocationID   string `json:"invocation_id"`
	Responsibility string `json:"responsibility"`
	Workspace      struct {
		Access string `json:"access"`
	} `json:"workspace"`
	Recovery *struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	} `json:"recovery"`
}

func main() {
	time.Sleep(100 * time.Millisecond)
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(20)
	}
	var prompt promptEnvelope
	if json.Unmarshal(body, &prompt) != nil {
		os.Exit(21)
	}
	family, brokerURL, token := brokerConfiguration()
	if family == "" || brokerURL == "" || token == "" {
		os.Exit(22)
	}
	resumeID, resume := explicitResume(family, os.Args[1:])
	statePath := filepath.Join(os.Getenv("HOME"), ".native-session-id")
	countPath := filepath.Join(os.Getenv("HOME"), ".native-resume-count")
	if resume {
		retained, readErr := os.ReadFile(statePath)
		if readErr != nil || string(retained) != sessionID ||
			resumeID != sessionID {
			os.Exit(23)
		}
		count, countErr := os.ReadFile(countPath)
		if countErr != nil || len(count) != 1 ||
			count[0] < '0' || count[0] >= '9' ||
			os.WriteFile(countPath, []byte{count[0] + 1}, 0o600) != nil {
			os.Exit(24)
		}
	} else {
		if os.WriteFile(statePath, []byte(sessionID), 0o600) != nil ||
			os.WriteFile(countPath, []byte("0"), 0o600) != nil {
			os.Exit(24)
		}
	}
	emitInit(family, prompt.Workspace.Access)
	initialize := map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name": "native-continuation", "version": "1.0.0",
		},
	}
	if status, _ := rpc(
		brokerURL,
		token,
		1,
		"initialize",
		initialize,
	); status != http.StatusOK {
		os.Exit(25)
	}
	if status, _ := notify(
		brokerURL,
		token,
		"notifications/initialized",
		map[string]any{},
	); status != http.StatusAccepted {
		os.Exit(26)
	}
	if status, _ := rpc(
		brokerURL,
		token,
		2,
		"tools/list",
		map[string]any{},
	); status != http.StatusOK {
		os.Exit(27)
	}
	if mode := credentialFixtureMode(family); mode != "" {
		switch mode {
		case "unreachable":
			time.Sleep(500 * time.Millisecond)
			os.Exit(29)
		case "unauthorized":
			os.Exit(2)
		case "exitone":
			os.Exit(1)
		case "expire":
			if os.WriteFile(
				credentialFixturePath(family),
				expiredClaudeCredentialFixture(),
				0o600,
			) != nil {
				os.Exit(24)
			}
			os.Exit(29)
		case "rotation":
			if os.WriteFile(
				credentialFixturePath(family),
				[]byte(`{"rotation_marker":"running"}`),
				0o600,
			) != nil {
				os.Exit(24)
			}
		}
	}
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(prompt.InvocationID, "prose-nudge-twice") ||
		(strings.Contains(prompt.InvocationID, "prose-nudge") &&
			prompt.Recovery == nil) {
		emitProse(family)
		return
	}
	if strings.Contains(prompt.InvocationID, "resume-prose") &&
		prompt.Recovery != nil &&
		prompt.Recovery.Kind != "nudge" {
		emitProse(family)
		return
	}
	if strings.Contains(prompt.InvocationID, "recoverable") &&
		(prompt.Recovery == nil ||
			(prompt.Recovery.Kind == "answer" &&
				strings.Contains(
					strings.ToLower(prompt.Recovery.Content),
					"yield again",
				))) {
		status, response := rpc(
			brokerURL,
			token,
			3,
			"tools/call",
			map[string]any{
				"name": "sworn_yield",
				"arguments": map[string]any{"yield": map[string]any{
					"schema_version": "sworn.yield/v1",
					"invocation_id":  prompt.InvocationID,
					"kind":           "question",
					"message":        "Provide the bounded fixture answer.",
				}},
			},
		)
		if status != http.StatusOK ||
			!bytes.Contains(response, []byte(`"text":"accepted"`)) {
			os.Exit(30)
		}
		select {}
	}
	submission := map[string]any{
		"schema_version": "sworn.submission/v1",
		"invocation_id":  prompt.InvocationID,
		"responsibility": prompt.Responsibility,
		"summary":        "Deterministic native continuation fixture.",
		"detail":         "Bounded fixture detail.\n",
	}
	if prompt.Responsibility == "implementer_implementation" ||
		prompt.Responsibility == "work_verification" {
		checks := []byte("checks\n")
		sum := sha256.Sum256(checks)
		submission["checks"] = map[string]any{
			"byte_count": len(checks),
			"digest":     "sha256:" + hex.EncodeToString(sum[:]),
			"bytes":      base64.StdEncoding.EncodeToString(checks),
		}
	}
	if prompt.Responsibility == "work_verification" {
		submission["decision"] = map[string]any{"outcome": "fail"}
	}
	status, response := rpc(
		brokerURL,
		token,
		3,
		"tools/call",
		map[string]any{
			"name":      "sworn_submit",
			"arguments": map[string]any{"submission": submission},
		},
	)
	if status != http.StatusOK ||
		!bytes.Contains(response, []byte(`"text":"accepted"`)) {
		os.Exit(28)
	}
	select {}
}

func emitProse(family string) {
	if family == "codex" {
		fmt.Println(`{"type":"item.completed","item":{"type":"agent_message","text":"I completed the work but forgot the terminal."}}`)
		return
	}
	fmt.Println(`{"type":"result","subtype":"success","result":"I completed the work but forgot the terminal."}`)
}

// credentialFixtureMode reads the family credential bound into the guest and
// reports a fixture mode the host test requested through it. Every mode runs
// only after the native identity event and the full broker handshake, so the
// terminal classification site (which requires nativeSeen and broker.Ready)
// is always reached exactly like the offline_provider pattern that predates
// this slice.
func credentialFixtureMode(family string) string {
	body, err := os.ReadFile(credentialFixturePath(family))
	if err != nil {
		return ""
	}
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	mode, _ := envelope["offline_provider"].(string)
	switch mode {
	case "unreachable", "unauthorized", "exitone", "expire", "rotation":
		return mode
	default:
		return ""
	}
}

// credentialFixturePath is the credential target the pinned family's config
// binds into the guest (CodexCredentialTarget / ClaudeCredentialTarget).
func credentialFixturePath(family string) string {
	if family == "codex" {
		return "/home/sworn/.codex/auth.json"
	}
	return "/home/sworn/.claude/.credentials.json"
}

// expiredClaudeCredentialFixture is the positively-expired Claude OAuth
// shape: a strict-positive millis expiry one minute in the past.
func expiredClaudeCredentialFixture() []byte {
	expired := time.Now().UnixMilli() - 60_000
	body, _ := json.Marshal(map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "expired-fixture",
			"refreshToken": "expired-fixture",
			"expiresAt":    expired,
		},
	})
	return body
}

func explicitResume(family string, arguments []string) (string, bool) {
	switch family {
	case "codex":
		for index := range arguments {
			if arguments[index] != "resume" {
				continue
			}
			if index < 3 ||
				arguments[index-2] != "-C" ||
				arguments[index-1] != "/workspace" {
				return "", true
			}
			for candidate := index + 1; candidate < len(arguments)-1; candidate++ {
				if arguments[candidate] == sessionID &&
					arguments[candidate+1] == "-" {
					return sessionID, true
				}
			}
			return "", true
		}
	case "claude":
		for index := 0; index+1 < len(arguments); index++ {
			if arguments[index] == "--resume" {
				return arguments[index+1], true
			}
		}
	}
	return "", false
}

func emitInit(family string, access string) {
	if family == "codex" {
		fmt.Printf(
			"{\"type\":\"thread.started\",\"thread_id\":%q}\n",
			sessionID,
		)
		return
	}
	tools := []any{
		"mcp__sworn__Bash",
		"mcp__sworn__Read",
		"mcp__sworn__Glob",
		"mcp__sworn__Grep",
	}
	if access == "read_write" {
		tools = append(
			tools,
			"mcp__sworn__Write",
			"mcp__sworn__Edit",
		)
	}
	tools = append(
		tools,
		"mcp__sworn__sworn_yield",
		"mcp__sworn__sworn_submit",
	)
	event := map[string]any{
		"type": "system", "subtype": "init",
		"session_id":     sessionID,
		"model":          "native-continuation-model",
		"permissionMode": "dontAsk",
		"slash_commands": []any{},
		"skills":         []any{},
		"plugins":        []any{},
		"tools":          tools,
		"mcp_servers": []any{map[string]any{
			"name": "sworn", "status": "connected",
		}},
		"capabilities": []any{
			"interrupt_receipt_v1",
			"interrupt_cancel_queued_v1",
			"msg_lifecycle_v1",
		},
		"analytics_disabled":        true,
		"product_feedback_disabled": true,
	}
	body, _ := json.Marshal(event)
	fmt.Println(string(body))
}

func brokerConfiguration() (string, string, string) {
	if body, err := os.ReadFile("/etc/codex/config.toml"); err == nil {
		var brokerURL, token string
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "url = ") {
				brokerURL = quoted(line)
			}
			if strings.HasPrefix(line, "http_headers = ") {
				start := strings.Index(line, "Bearer ")
				if start >= 0 {
					rest := line[start+len("Bearer "):]
					end := strings.Index(rest, "\"")
					if end >= 0 {
						token = rest[:end]
					}
				}
			}
		}
		return "codex", brokerURL, token
	}
	body, err := os.ReadFile("/sworn/config/mcp.json")
	if err != nil {
		return "", "", ""
	}
	var config map[string]any
	if json.Unmarshal(body, &config) != nil {
		return "", "", ""
	}
	servers, _ := config["mcpServers"].(map[string]any)
	server, _ := servers["sworn"].(map[string]any)
	headers, _ := server["headers"].(map[string]any)
	return "claude", text(server["url"]), strings.TrimPrefix(
		text(headers["Authorization"]),
		"Bearer ",
	)
}

func quoted(line string) string {
	start := strings.Index(line, "\"")
	if start < 0 {
		return ""
	}
	rest := line[start+1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func text(value any) string {
	body, _ := value.(string)
	return body
}

func rpc(
	url string,
	token string,
	id int,
	method string,
	params any,
) (int, []byte) {
	return post(url, token, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
}

func notify(
	url string,
	token string,
	method string,
	params any,
) (int, []byte) {
	return post(url, token, map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func post(
	url string,
	token string,
	value any,
) (int, []byte) {
	body, _ := json.Marshal(value)
	request, err := http.NewRequest(
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, nil
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	result, _ := io.ReadAll(response.Body)
	return response.StatusCode, result
}
