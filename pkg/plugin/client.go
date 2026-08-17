package plugin

import (
	"context"

	compute "google.golang.org/api/compute/v1"
)

type gcloudBackendServiceClient struct {
	compute *compute.Service
}

var _ backendServiceClient = (*gcloudBackendServiceClient)(nil)

// Get fetches the backend service from GCP.
func (c *gcloudBackendServiceClient) Get(ctx context.Context, cfg *GCloudTrafficRouting) (*compute.BackendService, error) {
	if cfg.Region != "" {
		return c.compute.RegionBackendServices.
			Get(cfg.Project, cfg.Region, cfg.BackendServiceName).
			Context(ctx).
			Do()
	}
	return c.compute.BackendServices.
		Get(cfg.Project, cfg.BackendServiceName).
		Context(ctx).
		Do()
}

// Update persists the backend service back to GCP.
func (c *gcloudBackendServiceClient) Update(ctx context.Context, cfg *GCloudTrafficRouting, svc *compute.BackendService) error {
	if cfg.Region != "" {
		_, err := c.compute.RegionBackendServices.
			Update(cfg.Project, cfg.Region, cfg.BackendServiceName, svc).
			Context(ctx).
			Do()
		return err
	}
	_, err := c.compute.BackendServices.
		Update(cfg.Project, cfg.BackendServiceName, svc).
		Context(ctx).
		Do()
	return err
}
