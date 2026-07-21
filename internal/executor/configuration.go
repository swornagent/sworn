package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
)

// executorConfigurationDigest binds the complete process configuration which
// can change contained execution. Invocation-specific inputs remain bound by
// their own request and workspace digests.
func executorConfigurationDigest(options Options) string {
	hasher := sha256.New()
	bind := func(name, value string) {
		_, _ = hasher.Write([]byte(strconv.Itoa(len(name))))
		_, _ = hasher.Write([]byte{':'})
		_, _ = hasher.Write([]byte(name))
		_, _ = hasher.Write([]byte(strconv.Itoa(len(value))))
		_, _ = hasher.Write([]byte{':'})
		_, _ = hasher.Write([]byte(value))
	}
	bind("schema", "sworn-executor-configuration-v3")
	bind("containment_policy", ContainmentPolicyVersion)
	bind("invocation_schema", InvocationSchemaVersion)
	bind("workspace_export_schema", WorkspaceExportSchemaVersion)
	bind("runtime_root", options.RuntimeRoot)
	bind("writable_root", options.WritableRoot)
	bind("bubblewrap", options.BubblewrapPath)
	bind("systemd_run", options.SystemdRunPath)
	bind("systemctl", options.SystemctlPath)
	for index, argument := range options.ShimArgv {
		bind("shim_argv_"+strconv.Itoa(index), argument)
	}
	bind("shim_argc", strconv.Itoa(len(options.ShimArgv)))
	limits := options.Limits
	bind("limit_runtime_ns", strconv.FormatInt(int64(limits.Runtime), 10))
	bind("limit_memory_bytes", strconv.FormatUint(limits.MemoryBytes, 10))
	bind("limit_swap_bytes", strconv.FormatUint(limits.SwapBytes, 10))
	bind("limit_tasks", strconv.FormatUint(limits.Tasks, 10))
	bind("limit_cpu_percent", strconv.FormatUint(limits.CPUPercent, 10))
	bind("limit_file_bytes", strconv.FormatUint(limits.FileBytes, 10))
	bind("limit_temp_bytes", strconv.FormatUint(limits.TempBytes, 10))
	bind("limit_home_bytes", strconv.FormatUint(limits.HomeBytes, 10))
	bind("limit_input_bytes", strconv.FormatUint(limits.InputBytes, 10))
	bind("limit_workspace_bytes", strconv.FormatUint(limits.WorkspaceBytes, 10))
	bind("limit_stdout_bytes", strconv.Itoa(limits.StdoutBytes))
	bind("limit_stderr_bytes", strconv.Itoa(limits.StderrBytes))
	environment := append([]string(nil), options.AllowedEnvironment...)
	sort.Strings(environment)
	last := ""
	count := 0
	for _, name := range environment {
		if count != 0 && name == last {
			continue
		}
		bind("allowed_environment_"+strconv.Itoa(count), name)
		last = name
		count++
	}
	bind("allowed_environment_count", strconv.Itoa(count))
	bind("allow_host_network", strconv.FormatBool(options.AllowHostNetwork))
	bind("allow_nested_sandbox", strconv.FormatBool(options.AllowNestedSandbox))
	bind("credential_file_target", CredentialFileTarget)
	bind("credential_file_maximum_bytes", strconv.FormatInt(maximumCredentialFileBytes, 10))
	bind("credential_file", options.CredentialFile)
	bind("allow_credential_file", strconv.FormatBool(options.AllowCredentialFile))
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}
