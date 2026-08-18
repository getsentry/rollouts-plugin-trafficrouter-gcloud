package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	rolloutsPlugin "github.com/argoproj/argo-rollouts/rollout/trafficrouting/plugin/rpc"
	pluginTypes "github.com/argoproj/argo-rollouts/utils/plugin/types"
	"github.com/sirupsen/logrus"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

type backendServiceClient interface {
	Get(ctx context.Context, cfg *GCloudTrafficRouting) (*compute.BackendService, error)
	Update(ctx context.Context, cfg *GCloudTrafficRouting, svc *compute.BackendService) error
}

type RpcPlugin struct {
	client backendServiceClient
	LogCtx *logrus.Entry
	IsTest bool
}

var _ rolloutsPlugin.TrafficRouterPlugin = (*RpcPlugin)(nil)

// InitPlugin initializes the plugin.
func (r *RpcPlugin) InitPlugin() pluginTypes.RpcError {
	if r.IsTest {
		return pluginTypes.RpcError{}
	}

	ctx := context.Background()
	computeService, err := compute.NewService(ctx, option.WithScopes(compute.CloudPlatformScope))
	if err != nil {
		return pluginTypes.RpcError{ErrorString: fmt.Sprintf("failed to create google cloud compute client: %v", err)}
	}
	r.client = &gcloudBackendServiceClient{compute: computeService}
	return pluginTypes.RpcError{}
}

// SetWeight sets canary network endpoint group capacity scaler
func (r *RpcPlugin) SetWeight(rollout *v1alpha1.Rollout, desiredWeight int32, _ []v1alpha1.WeightDestination) pluginTypes.RpcError {
	ctx := context.TODO()

	cfg, err := getPluginConfig(rollout)
	if err != nil {
		return pluginTypes.RpcError{ErrorString: err.Error()}
	}

	if desiredWeight < 0 || desiredWeight > 100 {
		return pluginTypes.RpcError{ErrorString: fmt.Sprintf("desiredWeight %d is out of range, must be between 0 and 100", desiredWeight)}
	}

	svc, err := r.client.Get(ctx, cfg)
	if err != nil {
		return pluginTypes.RpcError{ErrorString: fmt.Sprintf("failed to get backend service %q: %v", cfg.BackendServiceName, err)}
	}

	canaryScaler := float64(desiredWeight) / 100
	stableScaler := 1.0
	if cfg.UpdateStableCapacityScaler {
		stableScaler = 1.0 - canaryScaler
	}
	r.updateBackendService(cfg, svc, stableScaler, canaryScaler)

	r.LogCtx.WithFields(logrus.Fields{
		"backendService": cfg.BackendServiceName,
		"desiredWeight":  desiredWeight,
		"canaryScaler":   canaryScaler,
		"stableScaler":   stableScaler,
	}).Info("Updating backend service capacity scalers")

	if err := r.client.Update(ctx, cfg, svc); err != nil {
		return pluginTypes.RpcError{ErrorString: fmt.Sprintf("failed to update backend service %q: %v", cfg.BackendServiceName, err)}
	}

	return pluginTypes.RpcError{}
}

func (r *RpcPlugin) updateBackendService(cfg *GCloudTrafficRouting, svc *compute.BackendService, stableScaler, canaryScaler float64) {
	for _, backend := range svc.Backends {
		if strings.HasSuffix(backend.Group, cfg.CanaryNegPattern) {
			backend.CapacityScaler = canaryScaler
		}
		if strings.HasSuffix(backend.Group, cfg.StableNegPattern) {
			backend.CapacityScaler = stableScaler
		}
	}

}

// Type returns the type of the plugin.
func (r *RpcPlugin) Type() string {
	return Type
}

// UpdateHash is currently an empty stub to satisfy the interface. It is not
// meaningful for GCP backend service capacity scaling.
func (r *RpcPlugin) UpdateHash(_ *v1alpha1.Rollout, _, _ string, _ []v1alpha1.WeightDestination) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

// SetHeaderRoute is currently an empty stub to satisfy the interface. Header
// based routing is not supported by this plugin.
func (r *RpcPlugin) SetHeaderRoute(_ *v1alpha1.Rollout, _ *v1alpha1.SetHeaderRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

// VerifyWeight is currently an empty stub to satisfy the interface. Weight
// verification is not implemented for this plugin.
func (r *RpcPlugin) VerifyWeight(_ *v1alpha1.Rollout, _ int32, _ []v1alpha1.WeightDestination) (pluginTypes.RpcVerified, pluginTypes.RpcError) {
	return pluginTypes.NotImplemented, pluginTypes.RpcError{}
}

// SetMirrorRoute is currently an empty stub to satisfy the interface. Traffic
// mirroring is not supported by this plugin.
func (r *RpcPlugin) SetMirrorRoute(_ *v1alpha1.Rollout, _ *v1alpha1.SetMirrorRoute) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

// RemoveManagedRoutes is currently an empty stub to satisfy the interface. This
// plugin does not create managed routes.
func (r *RpcPlugin) RemoveManagedRoutes(_ *v1alpha1.Rollout) pluginTypes.RpcError {
	return pluginTypes.RpcError{}
}

func getPluginConfig(rollout *v1alpha1.Rollout) (*GCloudTrafficRouting, error) {
	cfg := GCloudTrafficRouting{}
	if err := json.Unmarshal(rollout.Spec.Strategy.Canary.TrafficRouting.Plugins[ConfigKey], &cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateConfig(cfg *GCloudTrafficRouting) error {
	var missing []string
	if cfg.Project == "" {
		missing = append(missing, "project")
	}
	if cfg.BackendServiceName == "" {
		missing = append(missing, "backendServiceName")
	}
	if cfg.CanaryNegPattern == "" {
		missing = append(missing, "canaryNegPattern")
	}
	if cfg.StableNegPattern == "" {
		missing = append(missing, "stableNegPattern")
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid gcloud traffic routing configuration, the following fields must be set: %s", strings.Join(missing, ", "))
	}
	return nil
}
