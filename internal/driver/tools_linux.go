//go:build linux

package driver

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"unicode/utf8"
)

func readToolPath(root, target string) ([]byte, error) {
	file, info, err := openToolNode(root, target, syscall.O_RDONLY, 0, false)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxToolResultBytes {
		if file != nil {
			_ = file.Close()
		}
		return nil, fail("TOOL_PATH_INVALID")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxToolResultBytes+1))
	if err != nil || len(body) > MaxToolResultBytes || !utf8.Valid(body) {
		clearBytes(body)
		return nil, fail("TOOL_READ_FAILED")
	}
	return body, nil
}

func writeToolPath(root, target string, body []byte) error {
	file, info, err := openToolNode(
		root,
		target,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC,
		0o600,
		true,
	)
	if err != nil || !info.Mode().IsRegular() {
		if file != nil {
			_ = file.Close()
		}
		return fail("TOOL_WRITE_FAILED")
	}
	written, writeErr := file.Write(body)
	closeErr := file.Close()
	if writeErr != nil || written != len(body) || closeErr != nil {
		return fail("TOOL_WRITE_FAILED")
	}
	return nil
}

func editToolPath(root, target string, oldBody, newBody []byte) error {
	file, info, err := openToolNode(root, target, syscall.O_RDWR, 0, false)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxToolResultBytes {
		if file != nil {
			_ = file.Close()
		}
		return fail("TOOL_EDIT_FAILED")
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxToolResultBytes+1))
	if err != nil || len(body) > MaxToolResultBytes || !utf8.Valid(body) ||
		bytes.Count(body, oldBody) != 1 {
		clearBytes(body)
		return fail("TOOL_EDIT_FAILED")
	}
	updated := bytes.Replace(body, oldBody, newBody, 1)
	clearBytes(body)
	defer clearBytes(updated)
	if len(updated) > MaxToolResultBytes ||
		file.Truncate(0) != nil {
		return fail("TOOL_EDIT_FAILED")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return fail("TOOL_EDIT_FAILED")
	}
	written, err := file.Write(updated)
	if err != nil || written != len(updated) {
		return fail("TOOL_EDIT_FAILED")
	}
	return nil
}

func listToolDirectory(root, target string) ([]toolPathEntry, error) {
	file, info, err := openToolNode(root, target, syscall.O_RDONLY, 0, false)
	if err != nil || !info.IsDir() {
		if file != nil {
			_ = file.Close()
		}
		return nil, fail("TOOL_PATH_INVALID")
	}
	defer file.Close()
	entries, _, err := walkToolDirectory(file, "", false, 0, 0)
	if err != nil {
		clearToolEntries(entries)
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Relative < entries[right].Relative
	})
	return entries, nil
}

func scanToolText(root, target string) ([]toolPathEntry, error) {
	file, info, err := openToolNode(root, target, syscall.O_RDONLY, 0, false)
	if err != nil {
		if file != nil {
			_ = file.Close()
		}
		return nil, fail("TOOL_PATH_INVALID")
	}
	defer file.Close()
	if info.Mode().IsRegular() {
		if info.Size() > MaxToolResultBytes {
			return nil, fail("RESOURCE_LIMIT")
		}
		body, readErr := io.ReadAll(io.LimitReader(file, MaxToolResultBytes+1))
		if readErr != nil || len(body) > MaxToolResultBytes {
			clearBytes(body)
			return nil, fail("TOOL_READ_FAILED")
		}
		if !utf8.Valid(body) {
			clearBytes(body)
			return nil, nil
		}
		return []toolPathEntry{{Body: body}}, nil
	}
	if !info.IsDir() {
		return nil, fail("TOOL_PATH_INVALID")
	}
	entries, _, err := walkToolDirectory(file, "", true, 0, 0)
	if err != nil {
		clearToolEntries(entries)
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Relative < entries[right].Relative
	})
	return entries, nil
}

func clearToolEntries(entries []toolPathEntry) {
	for index := range entries {
		clearBytes(entries[index].Body)
		entries[index].Body = nil
	}
}

func openToolNode(
	root string,
	target string,
	flags int,
	mode uint32,
	createParents bool,
) (*os.File, os.FileInfo, error) {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." ||
		len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		return nil, nil, fail("TOOL_PATH_INVALID")
	}
	components := []string{}
	if relative != "." {
		components = splitCleanPath(relative)
	}
	rootFD, err := syscall.Open(
		root,
		syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, nil, fail("TOOL_PATH_INVALID")
	}
	current := rootFD
	for _, component := range components[:maximum(0, len(components)-1)] {
		next, openErr := syscall.Openat(
			current,
			component,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
			0,
		)
		if openErr != nil && createParents && openErr == syscall.ENOENT {
			if mkdirErr := syscall.Mkdirat(current, component, 0o700); mkdirErr != nil &&
				mkdirErr != syscall.EEXIST {
				_ = syscall.Close(current)
				return nil, nil, fail("TOOL_PATH_INVALID")
			}
			next, openErr = syscall.Openat(
				current,
				component,
				syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
				0,
			)
		}
		_ = syscall.Close(current)
		if openErr != nil {
			return nil, nil, fail("TOOL_PATH_INVALID")
		}
		current = next
	}
	if len(components) == 0 {
		file := os.NewFile(uintptr(current), "sworn-tool-root")
		if file == nil {
			_ = syscall.Close(current)
			return nil, nil, fail("TOOL_PATH_INVALID")
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return nil, nil, fail("TOOL_PATH_INVALID")
		}
		return file, info, nil
	}
	descriptor, err := syscall.Openat(
		current,
		components[len(components)-1],
		flags|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,
		mode,
	)
	_ = syscall.Close(current)
	if err != nil {
		return nil, nil, fail("TOOL_PATH_INVALID")
	}
	file := os.NewFile(uintptr(descriptor), "sworn-tool-node")
	if file == nil {
		_ = syscall.Close(descriptor)
		return nil, nil, fail("TOOL_PATH_INVALID")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fail("TOOL_PATH_INVALID")
	}
	return file, info, nil
}

func walkToolDirectory(
	directory *os.File,
	prefix string,
	readBodies bool,
	count int,
	scanned int64,
) ([]toolPathEntry, int64, error) {
	children, err := directory.ReadDir(-1)
	if err != nil {
		return nil, scanned, fail("TOOL_READ_FAILED")
	}
	sort.Slice(children, func(left, right int) bool {
		return children[left].Name() < children[right].Name()
	})
	var entries []toolPathEntry
	for _, child := range children {
		count++
		if count > MaxToolWalkEntries {
			clearToolEntries(entries)
			return nil, scanned, fail("RESOURCE_LIMIT")
		}
		descriptor, openErr := syscall.Openat(
			int(directory.Fd()),
			child.Name(),
			syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,
			0,
		)
		if openErr != nil {
			clearToolEntries(entries)
			return nil, scanned, fail("TOOL_PATH_INVALID")
		}
		file := os.NewFile(uintptr(descriptor), "sworn-tool-walk")
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			clearToolEntries(entries)
			return nil, scanned, fail("TOOL_PATH_INVALID")
		}
		relative := filepath.ToSlash(filepath.Join(prefix, child.Name()))
		entry := toolPathEntry{Relative: relative, Directory: info.IsDir()}
		switch {
		case info.IsDir():
			nested, nestedScanned, nestedErr := walkToolDirectory(
				file,
				relative,
				readBodies,
				count,
				scanned,
			)
			_ = file.Close()
			if nestedErr != nil {
				clearToolEntries(entries)
				return nil, nestedScanned, nestedErr
			}
			count += len(nested)
			scanned = nestedScanned
			entries = append(entries, entry)
			entries = append(entries, nested...)
		case info.Mode().IsRegular():
			if readBodies {
				if info.Size() > MaxToolResultBytes ||
					scanned+info.Size() > MaxToolScanBytes {
					_ = file.Close()
					clearToolEntries(entries)
					return nil, scanned, fail("RESOURCE_LIMIT")
				}
				entry.Body, openErr = io.ReadAll(io.LimitReader(
					file,
					MaxToolResultBytes+1,
				))
				if openErr != nil || len(entry.Body) > MaxToolResultBytes {
					_ = file.Close()
					clearBytes(entry.Body)
					clearToolEntries(entries)
					return nil, scanned, fail("TOOL_READ_FAILED")
				}
				scanned += int64(len(entry.Body))
				if !utf8.Valid(entry.Body) {
					clearBytes(entry.Body)
					_ = file.Close()
					continue
				}
			}
			_ = file.Close()
			entries = append(entries, entry)
		default:
			_ = file.Close()
			clearToolEntries(entries)
			return nil, scanned, fail("TOOL_PATH_INVALID")
		}
	}
	return entries, scanned, nil
}

func splitCleanPath(relative string) []string {
	var components []string
	for relative != "." && relative != "" {
		directory, base := filepath.Split(relative)
		components = append([]string{base}, components...)
		relative = filepath.Clean(directory)
	}
	return components
}

func maximum(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func runToolBash(
	ctx context.Context,
	invocation Invocation,
	projectionRoot string,
	script string,
) ([]byte, error) {
	bwrap, err := trustedBubblewrap()
	if err != nil {
		return nil, err
	}
	workspace, err := openPinnedDirectory(invocation.HostWorkspace)
	if err != nil {
		return nil, err
	}
	defer workspace.Close()
	inputs, err := openPinnedDirectory(projectionRoot)
	if err != nil {
		return nil, err
	}
	defer inputs.Close()
	arguments := []string{
		"--die-with-parent", "--new-session",
		"--info-fd", "5",
		"--unshare-all", "--unshare-user", "--disable-userns",
		"--cap-drop", "ALL", "--clearenv",
		"--proc", "/proc", "--remount-ro", "/proc",
		"--dev", "/dev", "--tmpfs", "/tmp",
		"--dir", "/home", "--dir", "/home/sworn", "--dir", "/sworn",
		"--ro-bind", "/usr", "/usr",
	}
	for _, systemPath := range []string{"/lib", "/lib64"} {
		if _, statErr := os.Stat(systemPath); statErr == nil {
			arguments = append(arguments, "--ro-bind", systemPath, systemPath)
		}
	}
	if invocation.Request.Workspace.Access == ReadOnly {
		arguments = append(arguments, "--ro-bind-fd", "3", GuestWorkspacePath)
	} else {
		arguments = append(arguments, "--bind-fd", "3", GuestWorkspacePath)
	}
	for _, reserved := range []string{".git", ".baton", ".sworn"} {
		info, statErr := os.Lstat(filepath.Join(invocation.HostWorkspace, reserved))
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("UNSAFE_WORKSPACE_SURFACE")
		}
		target := filepath.Join(GuestWorkspacePath, reserved)
		if info.IsDir() {
			arguments = append(arguments, "--tmpfs", target, "--remount-ro", target)
		} else if info.Mode().IsRegular() {
			arguments = append(arguments, "--ro-bind", "/dev/null", target)
		} else {
			return nil, fail("UNSAFE_WORKSPACE_SURFACE")
		}
	}
	arguments = append(arguments,
		"--ro-bind-fd", "4", GuestInputPath,
		"--chdir", GuestWorkspacePath,
		"--setenv", "HOME", "/home/sworn",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "LC_ALL", "C.UTF-8",
		"--setenv", "TZ", "UTC",
		"/usr/bin/sh", "-eu", "-c", script,
	)
	command := exec.CommandContext(ctx, bwrap, arguments...)
	command.Env = []string{}
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return nil, fail("PROCESS_START_FAILED")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	command.ExtraFiles = []*os.File{workspace, inputs, statusWriter}
	command.SysProcAttr = linuxSandboxProcessAttributes()
	command.WaitDelay = processTerminationGrace
	var output boundedBuffer
	output.maximum = MaxBashCombinedOutput
	output.retain = MaxBashCombinedOutput
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		return nil, fail("PROCESS_START_FAILED")
	}
	_ = statusWriter.Close()
	_, group, statusErr := readSandboxProcessGroup(
		statusReader,
		command.Process.Pid,
	)
	_ = statusReader.Close()
	if statusErr != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
		return nil, fail("PROCESS_START_FAILED")
	}
	runErr := command.Wait()
	_ = syscall.Kill(-group, syscall.SIGTERM)
	groupErr := waitProcessGroup(group)
	body, _, overflow := output.snapshot()
	if overflow {
		return nil, fail("OUTPUT_OVERFLOW")
	}
	if groupErr != nil {
		clearBytes(body)
		return nil, groupErr
	}
	if runErr != nil {
		if isContextError(ctx.Err()) {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if !errorsAs(runErr, &exitErr) {
			return nil, fail("PROCESS_FAILED")
		}
		clearBytes(body)
		return nil, fail("PROCESS_FAILED")
	}
	return bytes.TrimSuffix(body, []byte("\n")), nil
}

func errorsAs(err error, target any) bool {
	switch typed := target.(type) {
	case **exec.ExitError:
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			*typed = exitErr
		}
		return ok
	default:
		return false
	}
}
