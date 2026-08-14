package driver

// This file records the single deterministic configuration migration that
// removes the vendor-named Mantle and DeepSeek adapter kinds. The production
// union in config.go never retains a vendor-named field; legacy documents are
// decoded through this shadow shape and rewritten into the wire-protocol-keyed
// union before admission. The mapping below names every rewritten field and is
// pinned byte-for-byte by TestLegacyMantleAndDeepSeekConfigsMigrateExactly.
//
// deepseek adapter -> openai adapter (one field rewrite):
//   key, id, version, endpoint, credential_header, credential_prefix,
//   credential_refs, response_bytes: carried unchanged
//   added api:                  "chat_completions"
//   added auth_mode:            "bearer"
//   added opaque_reasoning:     true        (opaque replay + reasoning_content)
//
// mantle adapter (api_key_bearer) -> openai adapter:
//   key, id, version, endpoint, credential_refs, response_bytes: carried
//   auth_mode "api_key_bearer" -> auth_mode "bearer"
//   added credential_header:    "Authorization"
//   added credential_prefix:    "Bearer "
//   added api:                  "chat_completions"
//
// mantle adapter (aws_chain_sigv4) -> openai adapter:
//   key, id, version, endpoint, credential_refs, response_bytes: carried
//   auth_mode "aws_chain_sigv4" -> auth_mode "aws_sigv4"
//   chain:                       carried unchanged
//   added api:                  "chat_completions"
//
// Profiles are untouched: they bind adapter keys and credential references,
// neither of which changes, so every prior-admitting profile admits unchanged.
// The migrated canonical bytes are the new configuration identity; a document
// that contains legacy entries is never compared byte-for-byte against its raw
// body, and a document without legacy entries keeps strict canonical semantics.

// legacyMantleAuthMode is the closed legacy authentication surface of a
// Bedrock Mantle adapter. It exists only inside the migration shadow shape.
type legacyMantleAuthMode string

const (
	legacyMantleAPIKey legacyMantleAuthMode = "api_key_bearer"
	legacyMantleAWS    legacyMantleAuthMode = "aws_chain_sigv4"
)

// legacyMantleProfileConfig mirrors the removed BedrockMantleProfileConfig
// solely so legacy documents decode before being rewritten.
type legacyMantleProfileConfig struct {
	Key            string               `json:"key"`
	ID             string               `json:"id"`
	Version        string               `json:"version"`
	Endpoint       string               `json:"endpoint"`
	CredentialRefs []string             `json:"credential_refs"`
	ResponseBytes  int                  `json:"response_bytes"`
	AuthMode       legacyMantleAuthMode `json:"auth_mode"`
	Chain          *AWSChainSpec        `json:"chain,omitempty"`
}

// legacyDriverAdapterConfig is the legacy-aware shadow of the production
// union. It is decode-only: vendor-named fields never enter the type system.
type legacyDriverAdapterConfig struct {
	Process  *DriverProcessAdapterConfig `json:"process,omitempty"`
	Native   *NativeAdapterConfig        `json:"native,omitempty"`
	OpenAI   *OpenAIProfileConfig        `json:"openai,omitempty"`
	DeepSeek *HTTPProfileConfig          `json:"deepseek,omitempty"`
	Gemini   *HTTPProfileConfig          `json:"gemini,omitempty"`
	Bedrock  *BedrockProfileConfig       `json:"bedrock,omitempty"`
	Mantle   *legacyMantleProfileConfig  `json:"mantle,omitempty"`
}

// legacyDriverConfig is the whole-document shadow shape. It accepts every new
// field plus the removed vendor-named adapter kinds.
type legacyDriverConfig struct {
	SchemaVersion string                      `json:"schema_version"`
	Credentials   []DriverCredentialSource    `json:"credentials"`
	Adapters      []legacyDriverAdapterConfig `json:"adapters"`
	Profiles      []DriverProfile             `json:"profiles"`
	Presets       []DriverPreset              `json:"presets,omitempty"`
}

func decodeLegacyDriverConfig(body []byte) (legacyDriverConfig, error) {
	var legacy legacyDriverConfig
	if _, err := decodeTyped(
		body,
		MaxDriverConfigBytes,
		[]string{"schema_version", "credentials", "adapters", "profiles"},
		[]string{"presets", "variables"},
		&legacy,
	); err != nil {
		return legacyDriverConfig{}, err
	}
	return legacy, nil
}

func migrateDriverConfig(legacy legacyDriverConfig) (DriverConfig, error) {
	config := DriverConfig{
		SchemaVersion: legacy.SchemaVersion,
		Credentials:   legacy.Credentials,
		Profiles:      legacy.Profiles,
		Presets:       legacy.Presets,
	}
	config.Adapters = make([]DriverAdapterConfig, 0, len(legacy.Adapters))
	for _, adapter := range legacy.Adapters {
		migrated, err := migrateAdapter(adapter)
		if err != nil {
			return DriverConfig{}, err
		}
		config.Adapters = append(config.Adapters, migrated)
	}
	return config, nil
}

func migrateAdapter(
	legacy legacyDriverAdapterConfig,
) (DriverAdapterConfig, error) {
	if legacy.DeepSeek != nil {
		source := legacy.DeepSeek
		opaque := true
		openai := cloneOpenAIProfileConfig(OpenAIProfileConfig{
			HTTPProfileConfig: *source,
			API:               OpenAIChatCompletionsAPI,
			AuthMode:          AuthModeBearer,
			OpaqueReasoning:   &opaque,
		})
		return DriverAdapterConfig{OpenAI: &openai}, nil
	}
	if legacy.Mantle != nil {
		source := legacy.Mantle
		config := OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key:            source.Key,
				ID:             source.ID,
				Version:        source.Version,
				Endpoint:       source.Endpoint,
				CredentialRefs: append([]string(nil), source.CredentialRefs...),
				ResponseBytes:  source.ResponseBytes,
			},
			API: OpenAIChatCompletionsAPI,
		}
		switch source.AuthMode {
		case legacyMantleAPIKey:
			config.AuthMode = AuthModeBearer
			config.CredentialHeader = "Authorization"
			config.CredentialPrefix = "Bearer "
		case legacyMantleAWS:
			if source.Chain == nil {
				return DriverAdapterConfig{}, fail("INVALID_DRIVER_CONFIG")
			}
			config.AuthMode = AuthModeAWSSigV4
			chain := cloneAWSChainSpec(*source.Chain)
			config.Chain = &chain
		default:
			return DriverAdapterConfig{}, fail("INVALID_DRIVER_CONFIG")
		}
		openai := cloneOpenAIProfileConfig(config)
		return DriverAdapterConfig{OpenAI: &openai}, nil
	}
	return DriverAdapterConfig{
		Process: legacy.Process,
		Native:  legacy.Native,
		OpenAI:  legacy.OpenAI,
		Gemini:  legacy.Gemini,
		Bedrock: legacy.Bedrock,
	}, nil
}
