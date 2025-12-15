/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DistributionStrategy defines how a model is distributed across endpoints
// +kubebuilder:validation:Enum=all;fixed;percentage
type DistributionStrategy string

const (
	// DistributionStrategyAll loads the model on all available endpoints (default, backward compatible)
	DistributionStrategyAll DistributionStrategy = "all"
	// DistributionStrategyFixed loads the model on a fixed number of endpoints
	DistributionStrategyFixed DistributionStrategy = "fixed"
	// DistributionStrategyPercentage loads the model on a percentage of available endpoints
	DistributionStrategyPercentage DistributionStrategy = "percentage"
)

// DistributionSpec configures how a model is distributed across endpoints
type DistributionSpec struct {
	// Strategy determines how endpoints are selected for this model
	// +kubebuilder:validation:Enum=all;fixed;percentage
	// +kubebuilder:default=all
	// +optional
	Strategy DistributionStrategy `json:"strategy,omitempty"`

	// Replicas is the number of endpoints to load this model on
	// Only used when Strategy is "fixed"
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Percentage of available endpoints to use (1-100)
	// Only used when Strategy is "percentage"
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	Percentage *int32 `json:"percentage,omitempty"`
}

// DynamoModelSpec defines the desired state of DynamoModel
type DynamoModelSpec struct {
	// ModelName is the full model identifier (e.g., "meta-llama/Llama-3.3-70B-Instruct-lora")
	// +kubebuilder:validation:Required
	ModelName string `json:"modelName"`

	// BaseModelName is the base model identifier that matches the service label
	// This is used to discover endpoints via headless services
	// +kubebuilder:validation:Required
	BaseModelName string `json:"baseModelName"`

	// ModelType specifies the type of model (e.g., "base", "lora", "adapter")
	// +kubebuilder:validation:Enum=base;lora;adapter
	// +kubebuilder:default=base
	// +optional
	ModelType string `json:"modelType,omitempty"`

	// Source specifies the model source location (only applicable for lora model type)
	// +optional
	Source *ModelSource `json:"source,omitempty"`

	// Distribution configures how this model is distributed across endpoints
	// If not specified, defaults to loading on all endpoints (backward compatible)
	// +optional
	Distribution *DistributionSpec `json:"distribution,omitempty"`
}

// ModelSource defines the source location of a model
type ModelSource struct {
	// URI is the model source URI
	// Supported formats:
	// - S3: s3://bucket/path/to/model
	// - HuggingFace: hf://org/model@revision_sha
	// +kubebuilder:validation:Required
	URI string `json:"uri"`
}

// EndpointInfo represents a single endpoint (pod) serving the model
type EndpointInfo struct {
	// Address is the full address of the endpoint (e.g., "http://10.0.1.5:9090")
	Address string `json:"address"`

	// PodName is the name of the pod serving this endpoint
	// +optional
	PodName string `json:"podName,omitempty"`

	// Ready indicates whether the endpoint is ready to serve traffic
	// For LoRA models: true if the POST /loras request succeeded with a 2xx status code
	// For base models: always false (no probing performed)
	Ready bool `json:"ready"`
}

// DynamoModelStatus defines the observed state of DynamoModel
type DynamoModelStatus struct {
	// Endpoints is the current list of all endpoints for this model
	// +optional
	Endpoints []EndpointInfo `json:"endpoints,omitempty"`

	// ReadyEndpoints is the count of endpoints that are ready
	ReadyEndpoints int `json:"readyEndpoints"`

	// TotalEndpoints is the total count of endpoints
	TotalEndpoints int `json:"totalEndpoints"`

	// TargetEndpoints is the list of pod names selected for this model based on distribution spec
	// When distribution is set, only these endpoints will have the model loaded
	// +optional
	TargetEndpoints []string `json:"targetEndpoints,omitempty"`

	// AvailableEndpoints is the total number of endpoints available for selection
	// This may be higher than TotalEndpoints when distribution limits the selection
	AvailableEndpoints int `json:"availableEndpoints,omitempty"`

	// Conditions represents the latest available observations of the model's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="BaseModel",type="string",JSONPath=".spec.baseModelName",description="Base model name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.modelType",description="Model type"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyEndpoints",description="Ready endpoints"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.totalEndpoints",description="Total endpoints"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:shortName=dm
// DynamoModel is the Schema for the dynamo models API
type DynamoModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamoModelSpec   `json:"spec,omitempty"`
	Status DynamoModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DynamoModelList contains a list of DynamoModel
type DynamoModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamoModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamoModel{}, &DynamoModelList{})
}

// IsLoRA returns true if this is a LoRA model (case-insensitive)
func (m *DynamoModel) IsLoRA() bool {
	return strings.EqualFold(m.Spec.ModelType, "lora")
}

// GetReadyEndpoints returns only the endpoints that are ready
func (m *DynamoModel) GetReadyEndpoints() []EndpointInfo {
	var ready []EndpointInfo
	for _, ep := range m.Status.Endpoints {
		if ep.Ready {
			ready = append(ready, ep)
		}
	}
	return ready
}

// HasEndpoints returns true if the model has any endpoints
func (m *DynamoModel) HasEndpoints() bool {
	return len(m.Status.Endpoints) > 0
}

// HasReadyEndpoints returns true if the model has any ready endpoints
func (m *DynamoModel) HasReadyEndpoints() bool {
	return m.Status.ReadyEndpoints > 0
}

// GetDistributionStrategy returns the effective distribution strategy
// Defaults to DistributionStrategyAll if not specified
func (m *DynamoModel) GetDistributionStrategy() DistributionStrategy {
	if m.Spec.Distribution == nil || m.Spec.Distribution.Strategy == "" {
		return DistributionStrategyAll
	}
	return m.Spec.Distribution.Strategy
}

// GetTargetReplicas returns the target number of endpoints based on distribution spec
// Takes the total available endpoints as input to calculate percentage-based targets
func (m *DynamoModel) GetTargetReplicas(availableEndpoints int) int {
	if m.Spec.Distribution == nil {
		return availableEndpoints
	}

	switch m.Spec.Distribution.Strategy {
	case DistributionStrategyFixed:
		if m.Spec.Distribution.Replicas != nil {
			target := int(*m.Spec.Distribution.Replicas)
			if target > availableEndpoints {
				return availableEndpoints
			}
			return target
		}
		return availableEndpoints
	case DistributionStrategyPercentage:
		if m.Spec.Distribution.Percentage != nil {
			target := availableEndpoints * int(*m.Spec.Distribution.Percentage) / 100
			if target < 1 {
				return 1
			}
			return target
		}
		return availableEndpoints
	default:
		return availableEndpoints
	}
}
