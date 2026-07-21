//go:build linux

package executor

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	shimCredentialBrokerArgument  = "--sworn-credential-broker"
	credentialBubblewrapFD        = bubblewrapStatusFD + 1
	credentialBrokerTokenBytes    = 32
	credentialBrokerSocketBytes   = 12
	credentialBrokerRequestPrefix = "sworn-credential-request-v1:"
	credentialBrokerResponse      = "sworn-credential-fd-v1"
	credentialBrokerIOTimeout     = 5 * time.Second
)

type credentialBrokerClient struct {
	runtimePath string
	socketName  string
	token       string
}

type credentialBroker struct {
	listener   *net.UnixListener
	socketPath string
	socketInfo os.FileInfo
	lease      *credentialFileLease
	clientData credentialBrokerClient
	done       chan error
	finishOnce sync.Once
	finishErr  error
}

func startCredentialBroker(runtimePath string, lease *credentialFileLease) (*credentialBroker, error) {
	if err := lease.validate(); err != nil {
		return nil, fmt.Errorf("validate credential before broker start: %w", err)
	}
	directory, err := openCredentialBrokerDirectory(runtimePath)
	if err != nil {
		return nil, err
	}
	defer directory.Close() //nolint:errcheck

	socketEntropy, err := randomCredentialBrokerValue(credentialBrokerSocketBytes)
	if err != nil {
		return nil, fmt.Errorf("generate credential broker socket name: %w", err)
	}
	token, err := randomCredentialBrokerValue(credentialBrokerTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate credential broker token: %w", err)
	}
	socketName := "credential-" + socketEntropy + ".sock"
	socketPath := filepath.Join(runtimePath, socketName)
	address := &net.UnixAddr{
		Name: credentialBrokerProcPath(directory.Fd(), socketName),
		Net:  "unixpacket",
	}
	listener, err := net.ListenUnix("unixpacket", address)
	if err != nil {
		return nil, fmt.Errorf("listen on credential broker socket: %w", err)
	}
	listener.SetUnlinkOnClose(false)
	cleanupListener := true
	defer func() {
		if cleanupListener {
			_ = listener.Close()
			_ = os.Remove(socketPath)
		}
	}()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return nil, fmt.Errorf("make credential broker socket private: %w", err)
	}
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		return nil, fmt.Errorf("inspect credential broker socket: %w", err)
	}
	if err := validateCredentialBrokerSocketInfo(socketInfo); err != nil {
		return nil, err
	}
	broker := &credentialBroker{
		listener:   listener,
		socketPath: socketPath,
		socketInfo: socketInfo,
		lease:      lease,
		clientData: credentialBrokerClient{
			runtimePath: runtimePath,
			socketName:  socketName,
			token:       token,
		},
		done: make(chan error, 1),
	}
	cleanupListener = false
	go func() { broker.done <- broker.serve() }()
	return broker, nil
}

func (broker *credentialBroker) client() credentialBrokerClient {
	if broker == nil {
		return credentialBrokerClient{}
	}
	return broker.clientData
}

func (broker *credentialBroker) finish() error {
	if broker == nil {
		return nil
	}
	broker.finishOnce.Do(func() {
		closeErr := broker.listener.Close()
		if errors.Is(closeErr, net.ErrClosed) {
			closeErr = nil
		}
		serveErr := <-broker.done
		removeErr := removeCredentialBrokerSocket(broker.socketPath, broker.socketInfo)
		broker.finishErr = errors.Join(closeErr, serveErr, removeErr)
	})
	return broker.finishErr
}

func (broker *credentialBroker) serve() error {
	connection, err := broker.listener.AcceptUnix()
	if err != nil {
		return fmt.Errorf("accept credential broker peer: %w", err)
	}
	_ = broker.listener.Close()
	defer connection.Close() //nolint:errcheck
	if err := connection.SetDeadline(time.Now().Add(credentialBrokerIOTimeout)); err != nil {
		return fmt.Errorf("bound credential broker deadline: %w", err)
	}
	if err := validateCredentialBrokerPeer(connection); err != nil {
		return err
	}

	expected := []byte(credentialBrokerRequestPrefix + broker.clientData.token)
	request := make([]byte, len(expected)+1)
	read, err := connection.Read(request)
	if err != nil {
		return fmt.Errorf("read credential broker request: %w", err)
	}
	if read != len(expected) || subtle.ConstantTimeCompare(request[:read], expected) != 1 {
		return errors.New("credential broker authentication failed")
	}
	oneMore := make([]byte, 1)
	read, err = connection.Read(oneMore)
	if read != 0 || !errors.Is(err, io.EOF) {
		return errors.New("credential broker request contained an extra message")
	}
	if err := broker.lease.validate(); err != nil {
		return fmt.Errorf("revalidate credential before descriptor transfer: %w", err)
	}
	control := syscall.UnixRights(int(broker.lease.file.Fd()))
	written, controlWritten, err := connection.WriteMsgUnix(
		[]byte(credentialBrokerResponse),
		control,
		nil,
	)
	if err != nil {
		return fmt.Errorf("transfer credential descriptor: %w", err)
	}
	if written != len(credentialBrokerResponse) || controlWritten != len(control) {
		return errors.New("credential broker descriptor transfer was incomplete")
	}
	return nil
}

func receiveCredentialFromBroker(client credentialBrokerClient) (*os.File, error) {
	if err := validateCredentialBrokerClient(client); err != nil {
		return nil, err
	}
	directory, err := openCredentialBrokerDirectory(client.runtimePath)
	if err != nil {
		return nil, err
	}
	defer directory.Close() //nolint:errcheck
	socketPath := filepath.Join(client.runtimePath, client.socketName)
	before, err := os.Lstat(socketPath)
	if err != nil {
		return nil, fmt.Errorf("inspect credential broker endpoint: %w", err)
	}
	if err := validateCredentialBrokerSocketInfo(before); err != nil {
		return nil, err
	}
	connection, err := net.DialUnix(
		"unixpacket",
		nil,
		&net.UnixAddr{
			Name: credentialBrokerProcPath(directory.Fd(), client.socketName),
			Net:  "unixpacket",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("connect to credential broker: %w", err)
	}
	defer connection.Close() //nolint:errcheck
	if err := connection.SetDeadline(time.Now().Add(credentialBrokerIOTimeout)); err != nil {
		return nil, fmt.Errorf("bound credential broker client deadline: %w", err)
	}
	after, err := os.Lstat(socketPath)
	if err != nil {
		return nil, fmt.Errorf("reinspect credential broker endpoint: %w", err)
	}
	if err := validateCredentialBrokerSocketInfo(after); err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, errors.New("credential broker endpoint identity changed")
	}
	request := []byte(credentialBrokerRequestPrefix + client.token)
	if written, err := connection.Write(request); err != nil || written != len(request) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return nil, fmt.Errorf("authenticate to credential broker: %w", err)
	}
	if err := connection.CloseWrite(); err != nil {
		return nil, fmt.Errorf("finish credential broker request: %w", err)
	}

	response := make([]byte, len(credentialBrokerResponse)+1)
	control := make([]byte, syscall.CmsgSpace(4))
	read, controlRead, flags, _, err := connection.ReadMsgUnix(response, control)
	if err != nil {
		return nil, fmt.Errorf("receive credential descriptor: %w", err)
	}
	descriptors, descriptorErr := parseCredentialBrokerDescriptors(control[:controlRead])
	keepDescriptor := false
	defer func() {
		if keepDescriptor {
			return
		}
		for _, descriptor := range descriptors {
			_ = syscall.Close(descriptor)
		}
	}()
	if read != len(credentialBrokerResponse) || string(response[:read]) != credentialBrokerResponse ||
		flags&(syscall.MSG_TRUNC|syscall.MSG_CTRUNC) != 0 {
		return nil, errors.New("credential broker response was invalid")
	}
	if descriptorErr != nil {
		return nil, errors.New("credential broker returned invalid descriptor control data")
	}
	if len(descriptors) != 1 {
		return nil, errors.New("credential broker did not return exactly one descriptor")
	}
	syscall.CloseOnExec(descriptors[0])
	flagsValue, _, flagsErr := syscall.Syscall(
		syscall.SYS_FCNTL,
		uintptr(descriptors[0]),
		uintptr(syscall.F_GETFL),
		0,
	)
	if flagsErr != 0 || int(flagsValue)&syscall.O_ACCMODE != syscall.O_RDWR {
		return nil, errors.New("credential broker descriptor is not writable")
	}
	credential := os.NewFile(uintptr(descriptors[0]), "sworn-credential")
	if credential == nil {
		return nil, errors.New("credential broker returned an invalid descriptor")
	}
	keepDescriptor = true
	info, err := credential.Stat()
	if err != nil {
		_ = credential.Close()
		return nil, fmt.Errorf("inspect brokered credential descriptor: %w", err)
	}
	if err := validateCredentialFileInfo(info); err != nil {
		_ = credential.Close()
		return nil, fmt.Errorf("validate brokered credential descriptor: %w", err)
	}
	return credential, nil
}

func parseCredentialBrokerDescriptors(control []byte) ([]int, error) {
	messages, err := syscall.ParseSocketControlMessage(control)
	if err != nil || len(messages) == 0 {
		return nil, errors.New("invalid descriptor control messages")
	}
	descriptors := make([]int, 0, len(messages))
	for index := range messages {
		messageDescriptors, err := syscall.ParseUnixRights(&messages[index])
		if err != nil {
			return descriptors, err
		}
		descriptors = append(descriptors, messageDescriptors...)
	}
	return descriptors, nil
}

func validateCredentialBrokerPeer(connection *net.UnixConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return fmt.Errorf("inspect credential broker peer socket: %w", err)
	}
	var peer *syscall.Ucred
	var peerErr error
	if err := raw.Control(func(descriptor uintptr) {
		peer, peerErr = syscall.GetsockoptUcred(int(descriptor), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return fmt.Errorf("control credential broker peer socket: %w", err)
	}
	if peerErr != nil {
		return fmt.Errorf("read credential broker peer identity: %w", peerErr)
	}
	if peer == nil || peer.Pid <= 0 || peer.Uid != uint32(os.Geteuid()) {
		return errors.New("credential broker peer is not the executor user")
	}
	return nil
}

func openCredentialBrokerDirectory(path string) (*os.File, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, '\x00') {
		return nil, errors.New("credential broker runtime must be a clean absolute path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve credential broker runtime: %w", err)
	}
	if resolved != path {
		return nil, errors.New("credential broker runtime contains a symbolic-link remap")
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect credential broker runtime: %w", err)
	}
	if err := validateCredentialParentInfo(before); err != nil {
		return nil, fmt.Errorf("validate credential broker runtime: %w", err)
	}
	descriptor, err := syscall.Open(
		path,
		syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open credential broker runtime: %w", err)
	}
	directory := os.NewFile(uintptr(descriptor), path)
	if directory == nil {
		_ = syscall.Close(descriptor)
		return nil, errors.New("open credential broker runtime: invalid file descriptor")
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("inspect opened credential broker runtime: %w", err)
	}
	if err := validateCredentialParentInfo(opened); err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("validate opened credential broker runtime: %w", err)
	}
	if !os.SameFile(before, opened) {
		_ = directory.Close()
		return nil, errors.New("credential broker runtime identity changed while opening")
	}
	return directory, nil
}

func validateCredentialBrokerSocketInfo(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("credential broker endpoint must be a Unix socket")
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("credential broker endpoint mode must be exactly 0600")
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || identity == nil || identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return errors.New("credential broker endpoint identity is invalid")
	}
	return nil
}

func validateCredentialBrokerClient(client credentialBrokerClient) error {
	if client.runtimePath == "" || !filepath.IsAbs(client.runtimePath) ||
		filepath.Clean(client.runtimePath) != client.runtimePath ||
		strings.ContainsRune(client.runtimePath, '\x00') {
		return errors.New("credential broker runtime must be a clean absolute path")
	}
	const socketPrefix = "credential-"
	const socketSuffix = ".sock"
	encodedSocketBytes := credentialBrokerSocketBytes * 2
	if len(client.socketName) != len(socketPrefix)+encodedSocketBytes+len(socketSuffix) ||
		!strings.HasPrefix(client.socketName, socketPrefix) ||
		!strings.HasSuffix(client.socketName, socketSuffix) ||
		filepath.Base(client.socketName) != client.socketName {
		return errors.New("credential broker socket name is invalid")
	}
	if _, err := hex.DecodeString(client.socketName[len(socketPrefix) : len(socketPrefix)+encodedSocketBytes]); err != nil {
		return errors.New("credential broker socket name is invalid")
	}
	if len(client.token) != credentialBrokerTokenBytes*2 {
		return errors.New("credential broker token is invalid")
	}
	if decoded, err := hex.DecodeString(client.token); err != nil || len(decoded) != credentialBrokerTokenBytes {
		return errors.New("credential broker token is invalid")
	}
	return nil
}

func credentialBrokerProcPath(directory uintptr, socketName string) string {
	return "/proc/self/fd/" + strconv.FormatUint(uint64(directory), 10) + "/" + socketName
}

func randomCredentialBrokerValue(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func removeCredentialBrokerSocket(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reinspect credential broker socket: %w", err)
	}
	if err := validateCredentialBrokerSocketInfo(current); err != nil {
		return err
	}
	if !os.SameFile(expected, current) {
		return errors.New("credential broker socket identity changed")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove credential broker socket: %w", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return fmt.Errorf("recheck removed credential broker socket: %w", err)
		}
		return errors.New("credential broker socket remains after removal")
	}
	return nil
}
