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

package modelendpoint

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ai-dynamo/dynamo/deploy/cloud/operator/api/v1alpha1"
)

func TestSelectTargetEndpoints(t *testing.T) {
	candidates := []Candidate{
		{Address: "http://10.0.1.1:9090", PodName: "worker-0"},
		{Address: "http://10.0.1.2:9090", PodName: "worker-1"},
		{Address: "http://10.0.1.3:9090", PodName: "worker-2"},
		{Address: "http://10.0.1.4:9090", PodName: "worker-3"},
	}

	tests := []struct {
		name          string
		model         *v1alpha1.DynamoModel
		expectedCount int
		expectStable  bool // Whether selection should be stable across calls
	}{
		{
			name: "nil distribution - returns all",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec:       v1alpha1.DynamoModelSpec{},
			},
			expectedCount: 4,
		},
		{
			name: "strategy all - returns all",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec: v1alpha1.DynamoModelSpec{
					Distribution: &v1alpha1.DistributionSpec{
						Strategy: v1alpha1.DistributionStrategyAll,
					},
				},
			},
			expectedCount: 4,
		},
		{
			name: "strategy fixed replicas=2",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec: v1alpha1.DynamoModelSpec{
					Distribution: &v1alpha1.DistributionSpec{
						Strategy: v1alpha1.DistributionStrategyFixed,
						Replicas: int32Ptr(2),
					},
				},
			},
			expectedCount: 2,
			expectStable:  true,
		},
		{
			name: "strategy fixed replicas exceeds available",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec: v1alpha1.DynamoModelSpec{
					Distribution: &v1alpha1.DistributionSpec{
						Strategy: v1alpha1.DistributionStrategyFixed,
						Replicas: int32Ptr(10),
					},
				},
			},
			expectedCount: 4, // Capped to available
		},
		{
			name: "strategy percentage 50%",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec: v1alpha1.DynamoModelSpec{
					Distribution: &v1alpha1.DistributionSpec{
						Strategy:   v1alpha1.DistributionStrategyPercentage,
						Percentage: int32Ptr(50),
					},
				},
			},
			expectedCount: 2, // 50% of 4
			expectStable:  true,
		},
		{
			name: "strategy percentage 25% - minimum 1",
			model: &v1alpha1.DynamoModel{
				ObjectMeta: metav1.ObjectMeta{Name: "test-model"},
				Spec: v1alpha1.DynamoModelSpec{
					Distribution: &v1alpha1.DistributionSpec{
						Strategy:   v1alpha1.DistributionStrategyPercentage,
						Percentage: int32Ptr(10), // 10% of 4 = 0.4, should round to 1
					},
				},
			},
			expectedCount: 1, // Minimum 1
			expectStable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectTargetEndpoints(tt.model, candidates)

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d endpoints, got %d", tt.expectedCount, len(result))
			}

			// Test stability - running twice should give same result
			if tt.expectStable {
				result2 := SelectTargetEndpoints(tt.model, candidates)
				if len(result) != len(result2) {
					t.Errorf("selection not stable: first %d, second %d", len(result), len(result2))
				}
				for i := range result {
					if result[i].PodName != result2[i].PodName {
						t.Errorf("selection not stable at index %d: first %s, second %s",
							i, result[i].PodName, result2[i].PodName)
					}
				}
			}
		})
	}
}

func TestSelectDeterministic(t *testing.T) {
	candidates := []Candidate{
		{Address: "http://10.0.1.1:9090", PodName: "worker-0"},
		{Address: "http://10.0.1.2:9090", PodName: "worker-1"},
		{Address: "http://10.0.1.3:9090", PodName: "worker-2"},
	}

	// Same model name should always select same endpoints
	result1 := selectDeterministic("model-a", candidates, 2)
	result2 := selectDeterministic("model-a", candidates, 2)

	if len(result1) != len(result2) {
		t.Fatalf("same model should select same count")
	}

	for i := range result1 {
		if result1[i].PodName != result2[i].PodName {
			t.Errorf("same model should select same endpoints")
		}
	}

	// Different model names should potentially select different endpoints
	// (not guaranteed, but with good hash distribution likely)
	resultA := selectDeterministic("model-a", candidates, 1)
	resultB := selectDeterministic("model-b", candidates, 1)
	resultC := selectDeterministic("model-c", candidates, 1)

	// At least two of three should be different (probabilistic but highly likely)
	allSame := resultA[0].PodName == resultB[0].PodName && resultB[0].PodName == resultC[0].PodName
	if allSame {
		t.Log("Warning: all three models selected same endpoint (unlikely but possible)")
	}
}

func TestExtractPodNames(t *testing.T) {
	candidates := []Candidate{
		{Address: "http://10.0.1.1:9090", PodName: "worker-0"},
		{Address: "http://10.0.1.2:9090", PodName: "worker-1"},
	}

	names := ExtractPodNames(candidates)
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "worker-0" || names[1] != "worker-1" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestFilterByPodNames(t *testing.T) {
	candidates := []Candidate{
		{Address: "http://10.0.1.1:9090", PodName: "worker-0"},
		{Address: "http://10.0.1.2:9090", PodName: "worker-1"},
		{Address: "http://10.0.1.3:9090", PodName: "worker-2"},
	}

	tests := []struct {
		name          string
		targetNames   []string
		expectedCount int
	}{
		{
			name:          "empty target - returns all",
			targetNames:   nil,
			expectedCount: 3,
		},
		{
			name:          "filter to subset",
			targetNames:   []string{"worker-0", "worker-2"},
			expectedCount: 2,
		},
		{
			name:          "filter to non-existent",
			targetNames:   []string{"worker-999"},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterByPodNames(candidates, tt.targetNames)
			if len(result) != tt.expectedCount {
				t.Errorf("expected %d, got %d", tt.expectedCount, len(result))
			}
		})
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}
