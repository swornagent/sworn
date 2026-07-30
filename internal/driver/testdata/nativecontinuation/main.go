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
	if resume {
		retained, readErr := os.ReadFile(statePath)
		if readErr != nil || string(retained) != sessionID ||
			resumeID != sessionID {
			os.Exit(23)
		}
	} else if os.WriteFile(statePath, []byte(sessionID), 0o600) != nil {
		os.Exit(24)
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
	if providerUnavailableFixture() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(29)
	}
	time.Sleep(100 * time.Millisecond)
	submission := map[string]any{
		"schema_version": "sworn.submission/v1",
		"invocation_id":  prompt.InvocationID,
		"responsibility": prompt.Responsibility,
		"summary":        "Deterministic native continuation fixture.",
		"detail":         "Bounded fixture detail.\n",
	}
	if prompt.Responsibility == "implementer_implementation" {
		checks := []byte("checks\n")
		sum := sha256.Sum256(checks)
		submission["checks"] = map[string]any{
			"byte_count": len(checks),
			"digest":     "sha256:" + hex.EncodeToString(sum[:]),
			"bytes":      base64.StdEncoding.EncodeToString(checks),
		}
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

func providerUnavailableFixture() bool {
	for _, pathValue := range []string{
		"/home/sworn/.codex/auth.json",
		"/home/sworn/.claude.json",
	} {
		body, err := os.ReadFile(pathValue)
		if err == nil &&
			bytes.Contains(body, []byte(`"offline_provider":"unreachable"`)) {
			return true
		}
	}
	return false
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
	tools = append(tools, "mcp__sworn__sworn_submit")
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
