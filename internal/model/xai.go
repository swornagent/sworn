// xai.go — S03-xai-driver: xAI (Grok) provider support. xAI is OpenAI
// chat/completions-compatible, so dispatch itself rides the shared OAI client
// (see the "xai" case in NewClient, provider.go). This file carries only the
// two xAI-specific tables that have no OAI-shared home: the pricing map
// (wired into PriceForModel, client.go) and the models/list catalog client
// (wired into catalogProviderDefs, catalog.go). No provider SDK (ADR-0007) —
// net/http + encoding/json only.
package model

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// xaiPricing maps xAI model IDs to USD per 1M tokens. Keyed on the bare model
// id (post-prefix) the way PriceForModel receives it, matching the
// per-provider-map convention (anthropicPricing/googlePricing/bedrockPricing).
//
// Prices are xAI's published Grok flagship API rate (docs.x.ai / x.ai pricing,
// 2026-07-12 snapshot: $3.00 / 1M input, $15.00 / 1M output). The slice
// requirement (AC-04) is a real non-zero entry so honest cost is
// CostSource=pricing-table, not "unknown"; re-confirm the exact per-1M rate at
// the next pricing snapshot (spec R-4, tracked: sworn#41 pricing-snapshot pass).
var xaiPricing = map[string]struct {
	inputPricePer1M  float64
	outputPricePer1M float64
}{
	"grok-4.5": {3.00, 15.00},
}

// catalogXAIBaseURL is the base for xAI's OpenAI-compatible models/list
// endpoint. Package-level var (not const) so tests can redirect it to an
// httptest fixture, restored via t.Cleanup — same pattern as the other
// catalog base URLs in catalog.go.
var catalogXAIBaseURL = "https://api.x.ai/v1"

// listXAIModels fetches xAI's model list. xAI's /models endpoint is the
// OpenAI-compatible bare-ID shape ({"data":[{"id":...}]}) with no explicit
// tool-support signal, so every entry annotates Unknown (AC-02 fail-closed) —
// identical handling to listOpenAIModels/listGroqModels. No completion or
// probe dispatch is made (AC-04): the request targets models/list only.
func listXAIModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogXAIBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("xai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.XAIKey)

	body, err := catalogDoGet(client, req, "xai")
	if err != nil {
		return nil, err
	}
	return parseCatalogBareIDList(body, "xai")
}
