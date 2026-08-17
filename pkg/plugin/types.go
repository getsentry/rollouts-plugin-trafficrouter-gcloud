// Package plugin implements the Argo Rollouts TrafficRouterPlugin interface
// for Google Cloud (GCP) backend services.
package plugin

// Type holds this plugin type, returned by the Type() method.
const Type = "GoogleCloud"

// ConfigKey is used to identify this plugin's configuration in the argo-rollouts
// configmap and in the Rollout resource under
// spec.strategy.canary.trafficRouting.plugins.
const ConfigKey = "sentry.io/gcloud"

// GCloudTrafficRouting represents the parameters required to configure the
// Google Cloud traffic routing plugin. It is provided by the user under the
// plugin config key in the Rollout resource.
//
// Example:
//
//	spec:
//	  strategy:
//	    canary:
//	      trafficRouting:
//	        plugins:
//	          sentry.io/gcloud:
//	            project: my-gcp-project
//	            region: us-central1
//	            backendServiceName: my-backend-service
//	            canaryNegPattern: my-canary-neg
//	            stableNegPattern: my-stable-neg
//	            updateStableCapacityScaler: true
type GCloudTrafficRouting struct {
	// Project is the GCP project ID that owns the backend service.
	Project string `json:"project" protobuf:"bytes,1,opt,name=project"`

	// Region is the region of a regional backend service. Leave empty for a
	// global backend service.
	Region string `json:"region,omitempty" protobuf:"bytes,2,opt,name=region"`

	// BackendServiceName is the name of the GCP backend service that holds the
	// canary and stable backends.
	BackendServiceName string `json:"backendServiceName" protobuf:"bytes,3,opt,name=backendServiceName"`
	CanaryNegPattern   string `json:"canaryNegPattern" protobuf:"bytes,4,opt,name=canaryNegPattern"`
	StableNegPattern   string `json:"stableNegPattern" protobuf:"bytes,5,opt,name=stableNegPattern"`

	// UpdateStableCapacityScaler indicates whether to update the stable capacity scaler.
	// If set to false, plugin will keep it untouched and only update canary group
	UpdateStableCapacityScaler bool `json:"updateStableCapacityScaler" protobuf:"bool,6,opt,name=updateStableCapacityScaler"`
}
