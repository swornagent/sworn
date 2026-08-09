package driver

import (
	"net/http"
	"net/url"
)

// BedrockMantleAuthMode is closed to the two authentication surfaces exposed
// by Bedrock Mantle. A configured mode is never substituted after failure.
type BedrockMantleAuthMode string

const (
	BedrockMantleAPIKey BedrockMantleAuthMode = "api_key_bearer"
	BedrockMantleAWS    BedrockMantleAuthMode = "aws_chain_sigv4"
)

func (mode BedrockMantleAuthMode) valid() bool {
	return mode == BedrockMantleAPIKey || mode == BedrockMantleAWS
}

// BedrockMantleProfileConfig describes one OpenAI-compatible Chat Completions
// endpoint. Endpoint is the exact POST URL; it is not a base URL. Chain is
// present only for standard AWS-chain/SigV4 mode; API-key mode always uses
// Authorization: Bearer.
type BedrockMantleProfileConfig struct {
	Key     string `json:"key"`
	ID      string `json:"id"`
	Version string `json:"version"`
	// Endpoint is the exact POST URL.
	Endpoint       string                `json:"endpoint"`
	CredentialRefs []string              `json:"credential_refs"`
	ResponseBytes  int                   `json:"response_bytes"`
	AuthMode       BedrockMantleAuthMode `json:"auth_mode"`
	Chain          *AWSChainSpec         `json:"chain,omitempty"`
}

// NewBedrockMantleAdapter composes the existing OpenAI conversation codec and
// shared provider tool loop with exactly one configured Bedrock auth mode.
// Config.Endpoint is sent as the exact POST URL.
func NewBedrockMantleAdapter(
	config BedrockMantleProfileConfig,
	headerResolver HeaderCredentialResolver,
	awsResolver AWSRuntimeResolver,
	probe ProfileLiveProbe,
	roundTripper http.RoundTripper,
) (Adapter, error) {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.ID) ||
		!versionPattern.MatchString(config.Version) ||
		validateMantleEndpoint(config.Endpoint) != nil ||
		config.ResponseBytes < 1 ||
		config.ResponseBytes > MaxProviderResponseBytes ||
		len(config.CredentialRefs) == 0 ||
		!config.AuthMode.valid() {
		return nil, fail("INVALID_ADAPTER")
	}

	var transport providerTransport
	switch config.AuthMode {
	case BedrockMantleAPIKey:
		if headerResolver == nil || awsResolver != nil || config.Chain != nil {
			return nil, fail("INVALID_ADAPTER")
		}
		httpTransport, err := newHTTPTransport(
			HTTPProfileConfig{
				Key:              config.Key,
				ID:               config.ID,
				Version:          config.Version,
				Endpoint:         config.Endpoint,
				CredentialHeader: "Authorization",
				CredentialPrefix: "Bearer ",
				CredentialRefs:   append([]string(nil), config.CredentialRefs...),
				ResponseBytes:    config.ResponseBytes,
			},
			headerResolver,
			probe,
			roundTripper,
		)
		if err != nil {
			return nil, err
		}
		config.CredentialRefs = append(
			[]string(nil),
			httpTransport.config.CredentialRefs...,
		)
		transport = httpTransport
	case BedrockMantleAWS:
		if headerResolver != nil || awsResolver == nil || config.Chain == nil {
			return nil, fail("INVALID_ADAPTER")
		}
		bedrockTransport, err := newBedrockTransport(
			BedrockProfileConfig{
				Key:            config.Key,
				ID:             config.ID,
				Version:        config.Version,
				Endpoint:       config.Endpoint,
				CredentialRefs: append([]string(nil), config.CredentialRefs...),
				ResponseBytes:  config.ResponseBytes,
				Chain:          *config.Chain,
			},
			ProfileSurfaceBedrockMantleChat,
			awsResolver,
			probe,
			roundTripper,
		)
		if err != nil {
			return nil, err
		}
		chain := bedrockTransport.config.Chain
		config.Chain = &chain
		config.CredentialRefs = append(
			[]string(nil),
			bedrockTransport.config.CredentialRefs...,
		)
		transport = bedrockTransport
	default:
		return nil, fail("INVALID_ADAPTER")
	}

	factory := func(
		prompt []byte,
		model string,
		tools []providerToolDefinition,
	) (providerConversation, error) {
		return newOpenAIConversation(
			config.Endpoint,
			model,
			tools,
			prompt,
			providerDialectMantleChat,
			"",
		)
	}
	return newLoopAdapter(
		config.Key,
		config.ID,
		config.Version,
		ProfileBedrock,
		ProfileSurfaceBedrockMantleChat,
		providerDialectMantleChat,
		config,
		factory,
		transport,
	)
}

func validateMantleEndpoint(value string) error {
	if validateEndpoint(value) != nil {
		return fail("INVALID_ENDPOINT")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.RawQuery != "" {
		return fail("INVALID_ENDPOINT")
	}
	return nil
}
