//go:build linux

package executor

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialBrokerTransfersExactRetainedDescriptor(t *testing.T) {
	credentialPath := newTestCredentialFile(t, "credential")
	lease, err := acquireCredentialFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })
	runtimePath := newTestCredentialBrokerRuntime(t, "runtime")
	broker, err := startCredentialBroker(runtimePath, lease)
	if err != nil {
		t.Fatal(err)
	}
	client := broker.client()
	received, err := receiveCredentialFromBroker(client)
	if err != nil {
		_ = broker.finish()
		t.Fatal(err)
	}
	defer received.Close() //nolint:errcheck
	if err := broker.finish(); err != nil {
		t.Fatal(err)
	}
	want, err := lease.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	got, err := received.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(want, got) {
		t.Fatal("broker returned a different credential inode")
	}
	assertCredentialBrokerSocketRemoved(t, client)
}

func TestCredentialBrokerRejectsInvalidAuthentication(t *testing.T) {
	lease := newTestCredentialLease(t)
	broker, err := startCredentialBroker(newTestCredentialBrokerRuntime(t, "runtime"), lease)
	if err != nil {
		t.Fatal(err)
	}
	client := broker.client()
	if client.token[0] == '0' {
		client.token = "1" + client.token[1:]
	} else {
		client.token = "0" + client.token[1:]
	}
	received, receiveErr := receiveCredentialFromBroker(client)
	if received != nil {
		_ = received.Close()
	}
	if receiveErr == nil {
		t.Fatal("invalid broker authentication received a descriptor")
	}
	finishErr := broker.finish()
	if finishErr == nil || !strings.Contains(finishErr.Error(), "authentication failed") {
		t.Fatalf("invalid-auth broker result = %v", finishErr)
	}
	assertCredentialBrokerSocketRemoved(t, broker.client())
}

func TestCredentialBrokerRejectsExtraMessage(t *testing.T) {
	lease := newTestCredentialLease(t)
	broker, err := startCredentialBroker(newTestCredentialBrokerRuntime(t, "runtime"), lease)
	if err != nil {
		t.Fatal(err)
	}
	client := broker.client()
	connection := dialCredentialBrokerForTest(t, client)
	request := []byte(credentialBrokerRequestPrefix + client.token)
	if written, err := connection.Write(request); err != nil || written != len(request) {
		t.Fatalf("write broker authentication: %d, %v", written, err)
	}
	if written, err := connection.Write([]byte("extra")); err != nil || written != len("extra") {
		t.Fatalf("write extra broker message: %d, %v", written, err)
	}
	if err := connection.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 1)
	_, _ = connection.Read(response)
	_ = connection.Close()
	finishErr := broker.finish()
	if finishErr == nil || !strings.Contains(finishErr.Error(), "extra message") {
		t.Fatalf("extra-message broker result = %v", finishErr)
	}
	assertCredentialBrokerSocketRemoved(t, client)
}

func TestCredentialBrokerSupportsLongRuntimePath(t *testing.T) {
	lease := newTestCredentialLease(t)
	runtimePath := newTestCredentialBrokerRuntime(
		t,
		filepath.Join(strings.Repeat("a", 90), strings.Repeat("b", 90)),
	)
	if len(filepath.Join(runtimePath, "credential-placeholder.sock")) <= 108 {
		t.Fatalf("test runtime path is not longer than AF_UNIX pathname limit: %q", runtimePath)
	}
	broker, err := startCredentialBroker(runtimePath, lease)
	if err != nil {
		t.Fatalf("start broker beneath long runtime path: %v", err)
	}
	client := broker.client()
	received, err := receiveCredentialFromBroker(client)
	if err != nil {
		_ = broker.finish()
		t.Fatalf("receive beneath long runtime path: %v", err)
	}
	_ = received.Close()
	if err := broker.finish(); err != nil {
		t.Fatalf("finish broker beneath long runtime path: %v", err)
	}
	assertCredentialBrokerSocketRemoved(t, client)
}

func dialCredentialBrokerForTest(t *testing.T, client credentialBrokerClient) *net.UnixConn {
	t.Helper()
	directory, err := openCredentialBrokerDirectory(client.runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialUnix(
		"unixpacket",
		nil,
		&net.UnixAddr{
			Name: credentialBrokerProcPath(directory.Fd(), client.socketName),
			Net:  "unixpacket",
		},
	)
	_ = directory.Close()
	if err != nil {
		t.Fatal(err)
	}
	return connection
}

func newTestCredentialLease(t *testing.T) *credentialFileLease {
	t.Helper()
	lease, err := acquireCredentialFile(newTestCredentialFile(t, "credential"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })
	return lease
}

func newTestCredentialBrokerRuntime(t *testing.T, suffix string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, suffix)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertCredentialBrokerSocketRemoved(t *testing.T, client credentialBrokerClient) {
	t.Helper()
	_, err := os.Lstat(filepath.Join(client.runtimePath, client.socketName))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential broker socket remains: %v", err)
	}
}
