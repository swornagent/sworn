//go:build !linux

package driver

import "context"
import "os"

func execAWSCommand(
	context.Context,
	AWSChainSpec,
	[][]byte,
	...string,
) ([]byte, error) {
	return nil, fail("UNSUPPORTED_HOST")
}

func openAWSClosure(AWSChainSpec) ([]*os.File, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
