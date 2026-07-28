package driver

import (
	"context"
	"os"
)

type FileCredentialResolver func(context.Context, string) (string, error)

type nativeCredentialLease interface {
	File() *os.File
	Close() error
}
