//go:build linux

package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type credentialFileLease struct {
	path       string
	file       *os.File
	parentPath string
	parent     *os.File
}

// withCredentialFile keeps the credential lifecycle identical for every
// admitted executor entry point. The retained descriptor stays exclusively
// locked until the systemd unit is proven quiescent; an unproven child keeps
// the inherited open-file description and therefore the lock.
func (executor *LinuxExecutor) withCredentialFile(
	invocation Invocation,
	execute func(*credentialFileLease) (RawCompletion, error),
) (completion RawCompletion, resultErr error) {
	if execute == nil {
		return RawCompletion{}, errors.New("credential-file execution callback is required")
	}
	credential, err := acquireCredentialFile(executor.options.CredentialFile)
	if err != nil {
		return RawCompletion{}, fmt.Errorf("acquire invocation credential file: %w", err)
	}
	defer func() {
		quiescenceContext, cancel := context.WithTimeout(context.Background(), shutdownGrace+2*time.Second)
		quiescenceErr := executor.waitUnitQuiescent(quiescenceContext, executor.unitName(invocation.ID))
		cancel()
		validationErr := credential.validate()
		releaseErr := finishCredentialFile(credential, quiescenceErr)
		if err := errors.Join(quiescenceErr, validationErr, releaseErr); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("revalidate invocation credential file: %w", err))
		}
	}()
	return execute(credential)
}

func validateConfiguredCredentialFile(options Options) error {
	if err := validateCredentialConfigurationShape(options); err != nil {
		return err
	}
	if !options.AllowCredentialFile {
		return nil
	}
	lease, err := acquireCredentialFile(options.CredentialFile)
	if err != nil {
		return err
	}
	return releaseCredentialFile(lease)
}

func acquireCredentialFile(path string) (*credentialFileLease, error) {
	if err := validateCredentialPath(path); err != nil {
		return nil, err
	}
	parentPath := filepath.Dir(path)
	parentBefore, err := os.Lstat(parentPath)
	if err != nil {
		return nil, fmt.Errorf("inspect credential directory: %w", err)
	}
	if err := validateCredentialParentInfo(parentBefore); err != nil {
		return nil, err
	}
	parentFD, err := syscall.Open(
		parentPath,
		syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open credential directory: %w", err)
	}
	parent := os.NewFile(uintptr(parentFD), parentPath)
	if parent == nil {
		_ = syscall.Close(parentFD)
		return nil, errors.New("open credential directory: invalid file descriptor")
	}
	parentOpened, err := parent.Stat()
	if err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("inspect opened credential directory: %w", err)
	}
	if err := validateCredentialParentInfo(parentOpened); err != nil {
		_ = parent.Close()
		return nil, err
	}
	if !os.SameFile(parentBefore, parentOpened) {
		_ = parent.Close()
		return nil, errors.New("credential directory path identity changed while opening")
	}
	before, err := os.Lstat(path)
	if err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("inspect credential file: %w", err)
	}
	if err := validateCredentialFileInfo(before); err != nil {
		_ = parent.Close()
		return nil, err
	}
	fd, err := syscall.Openat(
		parentFD,
		filepath.Base(path),
		syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("open credential file: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		_ = parent.Close()
		return nil, errors.New("open credential file: invalid file descriptor")
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = parent.Close()
		return nil, fmt.Errorf("inspect opened credential file: %w", err)
	}
	if err := validateCredentialFileInfo(opened); err != nil {
		_ = file.Close()
		_ = parent.Close()
		return nil, err
	}
	if !os.SameFile(before, opened) {
		_ = file.Close()
		_ = parent.Close()
		return nil, errors.New("credential file path identity changed while opening")
	}
	lease := &credentialFileLease{
		path:       path,
		file:       file,
		parentPath: parentPath,
		parent:     parent,
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = releaseCredentialFile(lease)
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errors.New("credential file is busy")
		}
		return nil, fmt.Errorf("lock credential file: %w", err)
	}
	if err := lease.validate(); err != nil {
		_ = releaseCredentialFile(lease)
		return nil, err
	}
	return lease, nil
}

func (lease *credentialFileLease) validate() error {
	if lease == nil || lease.file == nil || lease.parent == nil {
		return errors.New("credential file lease is invalid")
	}
	if err := validateCredentialPath(lease.path); err != nil {
		return err
	}
	currentParent, err := os.Lstat(lease.parentPath)
	if err != nil {
		return fmt.Errorf("reinspect credential directory: %w", err)
	}
	if err := validateCredentialParentInfo(currentParent); err != nil {
		return err
	}
	openedParent, err := lease.parent.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened credential directory: %w", err)
	}
	if err := validateCredentialParentInfo(openedParent); err != nil {
		return err
	}
	if !os.SameFile(currentParent, openedParent) {
		return errors.New("credential directory path identity changed")
	}
	current, err := os.Lstat(lease.path)
	if err != nil {
		return fmt.Errorf("reinspect credential file: %w", err)
	}
	if err := validateCredentialFileInfo(current); err != nil {
		return err
	}
	opened, err := lease.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened credential file: %w", err)
	}
	if err := validateCredentialFileInfo(opened); err != nil {
		return err
	}
	if !os.SameFile(current, opened) {
		return errors.New("credential file path identity changed")
	}
	return nil
}

func releaseCredentialFile(lease *credentialFileLease) error {
	return closeCredentialFile(lease, true)
}

// finishCredentialFile explicitly unlocks only after service quiescence is
// proven. SCM_RIGHTS preserves the credential file's open-file description,
// so closing this process's descriptor without LOCK_UN leaves the flock held
// for as long as an unproven child retains its inherited descriptor.
func finishCredentialFile(lease *credentialFileLease, quiescenceErr error) error {
	return closeCredentialFile(lease, quiescenceErr == nil)
}

func closeCredentialFile(lease *credentialFileLease, unlock bool) error {
	if lease == nil {
		return nil
	}
	var unlockErr, fileCloseErr, parentCloseErr error
	if lease.file != nil {
		if unlock {
			unlockErr = syscall.Flock(int(lease.file.Fd()), syscall.LOCK_UN)
		}
		fileCloseErr = lease.file.Close()
		lease.file = nil
	}
	if lease.parent != nil {
		parentCloseErr = lease.parent.Close()
		lease.parent = nil
	}
	if err := errors.Join(unlockErr, fileCloseErr, parentCloseErr); err != nil {
		return fmt.Errorf("release credential file: %w", err)
	}
	return nil
}

func validateCredentialPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("credential file must be a clean absolute path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve credential file: %w", err)
	}
	if resolved != path {
		return errors.New("credential file path contains a symbolic-link remap")
	}
	parentPath := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return fmt.Errorf("resolve credential directory: %w", err)
	}
	if resolvedParent != parentPath {
		return errors.New("credential directory path contains a symbolic-link remap")
	}
	return nil
}

func validateCredentialParentInfo(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("credential directory must be a non-symlink directory")
	}
	if info.Mode().Perm() != 0o700 {
		return errors.New("credential directory mode must be exactly 0700")
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || identity == nil {
		return errors.New("credential directory lacks a Linux identity")
	}
	if identity.Uid != uint32(os.Geteuid()) {
		return errors.New("credential directory must be owned by the executor user")
	}
	return nil
}

func validateCredentialFileInfo(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("credential file must be a non-symlink regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("credential file mode must be exactly 0600")
	}
	if info.Size() <= 0 || info.Size() > maximumCredentialFileBytes {
		return fmt.Errorf("credential file must contain 1 to %d bytes", maximumCredentialFileBytes)
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || identity == nil {
		return errors.New("credential file lacks a Linux identity")
	}
	if identity.Uid != uint32(os.Geteuid()) {
		return errors.New("credential file must be owned by the executor user")
	}
	if identity.Nlink != 1 {
		return errors.New("credential file must have exactly one hard link")
	}
	return nil
}
