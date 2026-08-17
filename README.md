# Argo Rollouts Traffic Router Plugin for Google Cloud

An [Argo Rollouts](https://argoproj.github.io/argo-rollouts/) traffic router
plugin that performs canary weight splitting by adjusting the
[`capacityScaler`](https://cloud.google.com/compute/docs/reference/rest/v1/backendServices)
of the backends of a Google Cloud (GCP) **Backend Service**.

The plugin implements the Argo Rollouts
[`TrafficRouterPlugin`](https://argoproj.github.io/argo-rollouts/plugins/#plugin-interfaces)
interface. Only `InitPlugin` and `SetWeight` are meaningful for GCP backend
service capacity scaling; the remaining interface methods are no-op stubs.

## How it works

A GCP Backend Service can reference multiple backends (instance groups or
network endpoint groups). Each backend has a `capacityScaler` in the range
`0.0`–`1.0` that scales the backend's serving capacity, which in turn controls
the proportion of traffic it receives.

Argo Rollouts expresses the canary weight as an integer percentage `0`–`100`.
On each `SetWeight` call the plugin translates that weight into a
`capacityScaler`:

```
canary.capacityScaler = desiredWeight / 100
stable.capacityScaler = (100 - desiredWeight) / 100
```

The canary and stable backends are matched by comparing the trailing segment of
each backend's `group` URL against the configured `canaryBackendName` and
`stableBackendName`.

A single logical backend often maps to **multiple** backend entries within one
backend service — one per region/zone (each pointing at a different
regional/zonal NEG or instance group that shares the same trailing name, e.g.
several `k8s-default-snuba-api-canary` NEGs across regions). The plugin updates
the `capacityScaler` on **all** matching backends, so every region receives the
same weight.

## Installation

The plugin runs as a sidecar-less child process of the Argo Rollouts
controller. Register it in the `argo-rollouts-config` ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: argo-rollouts
data:
  trafficRouterPlugins: |-
    - name: "argoproj-labs/gcloud"
      location: "file:///tmp/argo-rollouts/traffic-plugin"
      # or a remote artifact:
      # location: "https://github.com/argoproj-labs/rollouts-plugin-trafficrouter-gcloud/releases/download/v0.0.1/rollouts-plugin-trafficrouter-gcloud-linux-amd64"
```

The plugin authenticates to GCP using
[Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials),
so ensure the Argo Rollouts controller pod has access to a service account with
the `compute.backendServices.get` and `compute.backendServices.update`
permissions (for global backend services) or the regional equivalents.

## Rollout configuration

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: example-gcloud-rollout
spec:
  strategy:
    canary:
      canaryService: example-canary
      stableService: example-stable
      trafficRouting:
        plugins:
          argoproj-labs/gcloud:
            project: my-gcp-project
            region: us-central1            # omit for a global backend service
            backendServiceName: my-backend-service
            canaryBackendName: my-canary-neg
            stableBackendName: my-stable-neg
      steps:
        - setWeight: 20
        - pause: {}
        - setWeight: 50
        - pause: {}
        - setWeight: 100
```

### Configuration reference

| Field                | Required | Description                                                                                       |
| -------------------- | -------- | ------------------------------------------------------------------------------------------------- |
| `project`            | yes      | GCP project ID that owns the backend service.                                                     |
| `region`             | no       | Region of a regional backend service. Omit for a global backend service.                          |
| `backendServiceName` | yes      | Name of the GCP backend service holding the canary and stable backends.                           |
| `canaryBackendName`  | yes      | Trailing name of the backend (instance group / NEG) that receives canary traffic.                 |
| `stableBackendName`  | yes      | Trailing name of the backend (instance group / NEG) that receives stable traffic.                 |

## Building

```sh
make build              # builds dist/rollouts-plugin-trafficrouter-gcloud
make build-linux-amd64  # cross-compile for the controller image
make test               # run unit tests
```

## Development notes

Because plugin calls happen over RPC, the plugin does not rely on state stored
on the plugin struct between calls — each `SetWeight` re-reads the backend
service from GCP before mutating and persisting it.
