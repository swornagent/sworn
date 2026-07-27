//go:build !linux

package driver

import "context"

func platformInvokeNative(
	context.Context,
	Invocation,
	NativeAdapterConfig,
	string,
	nativeSurfaceCertificate,
) (Observation, error) {
	return Observation{}, fail("UNSUPPORTED_HOST")
}

func platformCaptureNativeSurface(
	context.Context,
	Invocation,
	NativeAdapterConfig,
) (nativeSurfaceCertificate, error) {
	return nativeSurfaceCertificate{}, fail("UNSUPPORTED_HOST")
}

func nativeVersion(context.Context, NativeAdapterConfig) ([]byte, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
