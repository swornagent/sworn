package driver

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestContinuationLifetimeKnobStampsExpiryFromLimits(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		millis int64
		want   time.Duration
	}{
		{name: "absent", millis: 0, want: 24 * time.Hour},
		{name: "one hour", millis: 3_600_000, want: time.Hour},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := newContinuationContractAdapter(
				"lifetime-stamp-"+strings.ReplaceAll(test.name, " ", "-"),
				"configuration-a",
			)
			design := continuationContractInvocation(
				t,
				adapter,
				"lifetime-stamp-"+strings.ReplaceAll(test.name, " ", "-"),
				RoleImplementer,
				ImplementerDesign,
				ReadOnly,
				true,
			)
			design.Request.Limits.MaxContinuationLifetimeMillis = test.millis
			permission, err := NewSubmissionPermission(
				design.Request,
				design.Selected,
				ContainmentReadOnly,
				ImplementerDesign,
			)
			if err != nil {
				t.Fatal(err)
			}
			design.Permission = permission
			before := time.Now()
			observation, handle, result, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				design,
				continuationContractBinding(),
				nil,
			)
			after := time.Now()
			if err != nil || observation.Handoff == nil || handle == nil ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"start = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			t.Cleanup(func() { _ = handle.Close() })
			cell := continuationCellFor(handle)
			cell.mu.Lock()
			expires := cell.expiresNano
			cell.mu.Unlock()
			wantMin := before.Add(test.want).UnixNano()
			wantMax := after.Add(test.want).UnixNano()
			if expires < wantMin || expires > wantMax {
				t.Fatalf(
					"expiresNano = %d, want [%d, %d] (%s from suspend)",
					expires,
					wantMin,
					wantMax,
					test.want,
				)
			}
		})
	}
}

func TestContinuationExpiryBoundaryIsInclusive(t *testing.T) {
	t.Parallel()
	adapter := newContinuationContractAdapter(
		"lifetime-boundary-adapter",
		"configuration-a",
	)
	handle, state := startContinuationFixture(t, adapter)
	cell := continuationCellFor(handle)
	cell.mu.Lock()
	cell.expiresNano = time.Now().UnixNano()
	cell.mu.Unlock()
	implementation := continuationContractInvocation(
		t,
		adapter,
		"lifetime-boundary-implementation",
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
		false,
	)
	_, _, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		continuationContractBinding(),
		handle,
	)
	_, _, closes := state.snapshot()
	if err != nil ||
		result.Mode != ContinuationModeFreshRehydrate ||
		result.Status != ContinuationStatusExpired ||
		result.Reason != "expiry" ||
		closes != 1 {
		t.Fatalf(
			"at-boundary expiry = result %#v, closes %d, error %v",
			result,
			closes,
			err,
		)
	}
}

func TestRequestContinuationLifetimeBounds(t *testing.T) {
	t.Parallel()
	request := contractRequest(t, RoleImplementer)
	request.Limits.MaxContinuationLifetimeMillis = 0
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("zero lifetime rejected: %v", err)
	}
	request.Limits.MaxContinuationLifetimeMillis = MaxContinuationLifetimeMillisLimit
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("cap lifetime rejected: %v", err)
	}
	request.Limits.MaxContinuationLifetimeMillis = -1
	if err := ValidateRequest(request); !IsCode(err, "INVALID_LIMIT") {
		t.Fatalf("negative lifetime error = %v, want INVALID_LIMIT", err)
	}
	request.Limits.MaxContinuationLifetimeMillis = MaxContinuationLifetimeMillisLimit + 1
	if err := ValidateRequest(request); !IsCode(err, "INVALID_LIMIT") {
		t.Fatalf("over-cap lifetime error = %v, want INVALID_LIMIT", err)
	}
	if (Limits{}).EffectiveContinuationLifetime() != 24*time.Hour {
		t.Fatalf(
			"default lifetime = %s, want 24h",
			(Limits{}).EffectiveContinuationLifetime(),
		)
	}
	if (Limits{MaxContinuationLifetimeMillis: 3_600_000}).EffectiveContinuationLifetime() != time.Hour {
		t.Fatalf("declared 1h lifetime did not bind")
	}
}
