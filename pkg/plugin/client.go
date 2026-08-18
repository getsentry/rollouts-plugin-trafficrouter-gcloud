package plugin

import (
	"context"
	"fmt"
	"strings"
	"time"

	compute "google.golang.org/api/compute/v1"
)

const operationPollInterval = 2 * time.Second

type gcloudBackendServiceClient struct {
	compute *compute.Service
}

var _ backendServiceClient = (*gcloudBackendServiceClient)(nil)

// backendServiceScope abstracts over the regional vs. global compute APIs,
// which are separate types with slightly different method signatures. scope()
// returns the right implementation based on whether cfg.Region is set, so the
// Get/Update/wait paths don't each need their own region branch.
type backendServiceScope interface {
	get(ctx context.Context) (*compute.BackendService, error)
	update(ctx context.Context, svc *compute.BackendService) (*compute.Operation, error)
	waitOperation(ctx context.Context, name string) (*compute.Operation, error)
}

func (c *gcloudBackendServiceClient) scope(cfg *GCloudTrafficRouting) backendServiceScope {
	if cfg.Region != "" {
		return regionalScope{compute: c.compute, cfg: cfg}
	}
	return globalScope{compute: c.compute, cfg: cfg}
}

type globalScope struct {
	compute *compute.Service
	cfg     *GCloudTrafficRouting
}

func (s globalScope) get(ctx context.Context) (*compute.BackendService, error) {
	return s.compute.BackendServices.Get(s.cfg.Project, s.cfg.BackendServiceName).Context(ctx).Do()
}

func (s globalScope) update(ctx context.Context, svc *compute.BackendService) (*compute.Operation, error) {
	return s.compute.BackendServices.Update(s.cfg.Project, s.cfg.BackendServiceName, svc).Context(ctx).Do()
}

func (s globalScope) waitOperation(ctx context.Context, name string) (*compute.Operation, error) {
	return s.compute.GlobalOperations.Wait(s.cfg.Project, name).Context(ctx).Do()
}

type regionalScope struct {
	compute *compute.Service
	cfg     *GCloudTrafficRouting
}

func (s regionalScope) get(ctx context.Context) (*compute.BackendService, error) {
	return s.compute.RegionBackendServices.Get(s.cfg.Project, s.cfg.Region, s.cfg.BackendServiceName).Context(ctx).Do()
}

func (s regionalScope) update(ctx context.Context, svc *compute.BackendService) (*compute.Operation, error) {
	return s.compute.RegionBackendServices.Update(s.cfg.Project, s.cfg.Region, s.cfg.BackendServiceName, svc).Context(ctx).Do()
}

func (s regionalScope) waitOperation(ctx context.Context, name string) (*compute.Operation, error) {
	return s.compute.RegionOperations.Wait(s.cfg.Project, s.cfg.Region, name).Context(ctx).Do()
}

// Get fetches the backend service from GCP.
func (c *gcloudBackendServiceClient) Get(ctx context.Context, cfg *GCloudTrafficRouting) (*compute.BackendService, error) {
	return c.scope(cfg).get(ctx)
}

// Update persists the backend service back to GCP and waits for the resulting
// operation to reach DONE. Backend service updates are asynchronous and can
// take ~90s to reconcile (Traffic Director); waiting serializes updates so the
// next SetWeight does not collide with an in-flight one and get a 400
// "resourceNotReady".
func (c *gcloudBackendServiceClient) Update(ctx context.Context, cfg *GCloudTrafficRouting, svc *compute.BackendService) error {
	scope := c.scope(cfg)
	op, err := scope.update(ctx, svc)
	if err != nil {
		return err
	}
	return waitOperation(ctx, scope, op)
}

// waitOperation blocks until op reaches DONE, then surfaces any
// operation-level error.
func waitOperation(ctx context.Context, scope backendServiceScope, op *compute.Operation) error {
	for op != nil && op.Status != "DONE" {
		// Operations.Wait long-polls but may return before the operation is
		// DONE, in which case we poll again after a short delay.
		waited, err := scope.waitOperation(ctx, op.Name)
		if err != nil {
			return err
		}
		if waited.Status != "DONE" {
			if err := sleepCtx(ctx, operationPollInterval); err != nil {
				return err
			}
		}
		op = waited
	}
	return operationError(op)
}

// operationError converts a failed compute operation into a Go error.
func operationError(op *compute.Operation) error {
	if op == nil || op.Error == nil || len(op.Error.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(op.Error.Errors))
	for _, e := range op.Error.Errors {
		msgs = append(msgs, fmt.Sprintf("%s: %s", e.Code, e.Message))
	}
	return fmt.Errorf("operation %s failed: %s", op.Name, strings.Join(msgs, "; "))
}

// sleepCtx sleeps for d unless ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
