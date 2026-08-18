package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	compute "google.golang.org/api/compute/v1"
)

type fakeBackendServiceClient struct {
	svc       *compute.BackendService
	getErr    error
	updateErr error

	updated *compute.BackendService
}

func (f *fakeBackendServiceClient) Get(_ context.Context, _ *GCloudTrafficRouting) (*compute.BackendService, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.svc, nil
}

func (f *fakeBackendServiceClient) Update(_ context.Context, _ *GCloudTrafficRouting, svc *compute.BackendService) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = svc
	return nil
}

func newRollout(cfg GCloudTrafficRouting) *v1alpha1.Rollout {
	raw, _ := json.Marshal(cfg)
	return &v1alpha1.Rollout{
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					TrafficRouting: &v1alpha1.RolloutTrafficRouting{
						Plugins: map[string]json.RawMessage{
							ConfigKey: raw,
						},
					},
				},
			},
		},
	}
}

func testConfig() GCloudTrafficRouting {
	return GCloudTrafficRouting{
		Project:                    "my-project",
		Region:                     "us-central1",
		BackendServiceName:         "my-backend-service",
		CanaryNegPattern:           "canary-neg",
		StableNegPattern:           "stable-neg",
		UpdateStableCapacityScaler: true,
	}
}

func newRpcPlugin(client backendServiceClient) *RpcPlugin {
	return &RpcPlugin{
		LogCtx: logrus.WithFields(logrus.Fields{"test": "true"}),
		IsTest: true,
		client: client,
	}
}

func findBackend(svc *compute.BackendService, suffix string) *compute.Backend {
	for _, b := range svc.Backends {
		if strings.HasSuffix(b.Group, suffix) {
			return b
		}
	}
	return nil
}

func TestSetWeight(t *testing.T) {
	cfg := testConfig()
	fake := &fakeBackendServiceClient{
		svc: &compute.BackendService{
			Name: cfg.BackendServiceName,
			Backends: []*compute.Backend{
				{Group: "https://compute/v1/projects/p/zones/z/networkEndpointGroups/canary-neg", CapacityScaler: 0.0},
				{Group: "https://compute/v1/projects/p/zones/z/networkEndpointGroups/stable-neg", CapacityScaler: 1.0},
			},
		},
	}
	r := newRpcPlugin(fake)

	rpcErr := r.SetWeight(newRollout(cfg), 30, nil)
	require.Empty(t, rpcErr.ErrorString)
	require.NotNil(t, fake.updated)

	canary := findBackend(fake.updated, "canary-neg")
	stable := findBackend(fake.updated, "stable-neg")
	require.NotNil(t, canary)
	require.NotNil(t, stable)
	assert.InDelta(t, 0.30, canary.CapacityScaler, 1e-9)
	assert.InDelta(t, 0.70, stable.CapacityScaler, 1e-9)
}

func TestSetWeightStableUntouchedByDefault(t *testing.T) {
	cfg := testConfig()
	cfg.UpdateStableCapacityScaler = false
	fake := &fakeBackendServiceClient{
		svc: &compute.BackendService{
			Name: cfg.BackendServiceName,
			Backends: []*compute.Backend{
				{Group: "x/canary-neg", CapacityScaler: 0.0},
				{Group: "x/stable-neg", CapacityScaler: 1.0},
			},
		},
	}
	r := newRpcPlugin(fake)

	rpcErr := r.SetWeight(newRollout(cfg), 30, nil)
	require.Empty(t, rpcErr.ErrorString)
	require.NotNil(t, fake.updated)

	assert.InDelta(t, 0.30, findBackend(fake.updated, "canary-neg").CapacityScaler, 1e-9)
	assert.InDelta(t, 1.0, findBackend(fake.updated, "stable-neg").CapacityScaler, 1e-9)
}

func TestSetWeightMultipleRegionalBackends(t *testing.T) {
	cfg := GCloudTrafficRouting{
		Project:                    "sentry-us2",
		BackendServiceName:         "snuba-api-0-http-backend-service-v2",
		CanaryNegPattern:           "k8s-default-snuba-api-canary",
		StableNegPattern:           "k8s-default-snuba-api",
		UpdateStableCapacityScaler: true,
	}
	fake := &fakeBackendServiceClient{
		svc: &compute.BackendService{
			Name: cfg.BackendServiceName,
			Backends: []*compute.Backend{
				{Group: "projects/sentry-us2/zones/us-central1-a/networkEndpointGroups/k8s-default-snuba-api-canary"},
				{Group: "projects/sentry-us2/zones/us-east1-b/networkEndpointGroups/k8s-default-snuba-api-canary"},
				{Group: "projects/sentry-us2/zones/europe-west1-c/networkEndpointGroups/k8s-default-snuba-api-canary"},
				{Group: "projects/sentry-us2/zones/us-central1-a/networkEndpointGroups/k8s-default-snuba-api"},
				{Group: "projects/sentry-us2/zones/us-east1-b/networkEndpointGroups/k8s-default-snuba-api"},
				{Group: "projects/sentry-us2/zones/europe-west1-c/networkEndpointGroups/k8s-default-snuba-api"},
			},
		},
	}
	r := newRpcPlugin(fake)

	rpcErr := r.SetWeight(newRollout(cfg), 40, nil)
	require.Empty(t, rpcErr.ErrorString)
	require.NotNil(t, fake.updated)

	var canaryCount, stableCount int
	for _, b := range fake.updated.Backends {
		switch {
		case strings.HasSuffix(b.Group, cfg.CanaryNegPattern):
			canaryCount++
			assert.InDelta(t, 0.40, b.CapacityScaler, 1e-9)
		case strings.HasSuffix(b.Group, cfg.StableNegPattern):
			stableCount++
			assert.InDelta(t, 0.60, b.CapacityScaler, 1e-9)
		}
	}
	assert.Equal(t, 3, canaryCount)
	assert.Equal(t, 3, stableCount)
}

func TestSetWeightZeroAndFull(t *testing.T) {
	cfg := testConfig()
	for _, tc := range []struct {
		weight     int32
		wantCanary float64
		wantStable float64
	}{
		{0, 0.0, 1.0},
		{100, 1.0, 0.0},
	} {
		fake := &fakeBackendServiceClient{
			svc: &compute.BackendService{
				Name: cfg.BackendServiceName,
				Backends: []*compute.Backend{
					{Group: "x/canary-neg"},
					{Group: "x/stable-neg"},
				},
			},
		}
		r := newRpcPlugin(fake)
		rpcErr := r.SetWeight(newRollout(cfg), tc.weight, nil)
		require.Empty(t, rpcErr.ErrorString)

		assert.InDelta(t, tc.wantCanary, findBackend(fake.updated, "canary-neg").CapacityScaler, 1e-9, "weight=%d", tc.weight)
		assert.InDelta(t, tc.wantStable, findBackend(fake.updated, "stable-neg").CapacityScaler, 1e-9, "weight=%d", tc.weight)
	}
}

func TestSetWeightUpdateErrorSurfaces(t *testing.T) {
	cfg := testConfig()
	fake := &fakeBackendServiceClient{
		svc: &compute.BackendService{
			Name: cfg.BackendServiceName,
			Backends: []*compute.Backend{
				{Group: "x/canary-neg"},
				{Group: "x/stable-neg"},
			},
		},
		updateErr: fmt.Errorf("boom"),
	}
	r := newRpcPlugin(fake)

	rpcErr := r.SetWeight(newRollout(cfg), 30, nil)
	require.NotEmpty(t, rpcErr.ErrorString)
	assert.Contains(t, rpcErr.ErrorString, "boom")
}

func TestOperationError(t *testing.T) {
	assert.NoError(t, operationError(nil))
	assert.NoError(t, operationError(&compute.Operation{Name: "op"}))
	assert.NoError(t, operationError(&compute.Operation{Name: "op", Error: &compute.OperationError{}}))

	err := operationError(&compute.Operation{
		Name: "op-1",
		Error: &compute.OperationError{
			Errors: []*compute.OperationErrorErrors{
				{Code: "RESOURCE_NOT_READY", Message: "not ready"},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "op-1")
	assert.Contains(t, err.Error(), "RESOURCE_NOT_READY")
	assert.Contains(t, err.Error(), "not ready")
}

func TestSetWeightOutOfRange(t *testing.T) {
	cfg := testConfig()
	r := newRpcPlugin(&fakeBackendServiceClient{})
	for _, w := range []int32{-1, 101} {
		rpcErr := r.SetWeight(newRollout(cfg), w, nil)
		assert.NotEmpty(t, rpcErr.ErrorString, "weight=%d should error", w)
	}
}

func TestGetPluginConfigValidation(t *testing.T) {
	_, err := getPluginConfig(newRollout(GCloudTrafficRouting{Project: "p"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backendServiceName")

	got, err := getPluginConfig(newRollout(testConfig()))
	require.NoError(t, err)
	assert.Equal(t, "my-backend-service", got.BackendServiceName)
}

func TestStubsReturnNoError(t *testing.T) {
	r := newRpcPlugin(&fakeBackendServiceClient{})
	assert.Empty(t, r.UpdateHash(nil, "", "", nil).ErrorString)
	assert.Empty(t, r.SetHeaderRoute(nil, nil).ErrorString)
	assert.Empty(t, r.SetMirrorRoute(nil, nil).ErrorString)
	assert.Empty(t, r.RemoveManagedRoutes(nil).ErrorString)
	assert.Equal(t, Type, r.Type())

	verified, rpcErr := r.VerifyWeight(nil, 0, nil)
	assert.Empty(t, rpcErr.ErrorString)
	assert.NotNil(t, verified)
}
