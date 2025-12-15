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
	"hash/fnv"
	"sort"

	"github.com/ai-dynamo/dynamo/deploy/cloud/operator/api/v1alpha1"
)

// SelectTargetEndpoints selects a subset of endpoints based on the model's distribution spec
// Uses deterministic selection to ensure stability across reconciliations
func SelectTargetEndpoints(model *v1alpha1.DynamoModel, candidates []Candidate) []Candidate {
	if len(candidates) == 0 {
		return candidates
	}

	targetCount := model.GetTargetReplicas(len(candidates))

	// If targeting all endpoints, return as-is
	if targetCount >= len(candidates) {
		return candidates
	}

	// Use deterministic selection based on model name
	return selectDeterministic(model.Name, candidates, targetCount)
}

// selectDeterministic selects N endpoints deterministically using consistent hashing
// This ensures the same endpoints are selected across reconciliations as long as the
// candidate list remains stable
func selectDeterministic(modelName string, candidates []Candidate, n int) []Candidate {
	if n >= len(candidates) {
		return candidates
	}
	if n <= 0 {
		return nil
	}

	// Score each candidate by hashing modelName + podName
	type scoredCandidate struct {
		candidate Candidate
		score     uint64
	}

	scored := make([]scoredCandidate, len(candidates))
	for i, c := range candidates {
		h := fnv.New64a()
		// Use modelName:podName to create a deterministic score
		h.Write([]byte(modelName + ":" + c.PodName))
		scored[i] = scoredCandidate{
			candidate: c,
			score:     h.Sum64(),
		}
	}

	// Sort by score for deterministic selection
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	// Select top N
	result := make([]Candidate, n)
	for i := 0; i < n; i++ {
		result[i] = scored[i].candidate
	}
	return result
}

// ExtractPodNames extracts pod names from candidates
func ExtractPodNames(candidates []Candidate) []string {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.PodName
	}
	return names
}

// FilterByPodNames filters candidates to only those with matching pod names
// This is used to ensure consistency when some endpoints have been removed
func FilterByPodNames(candidates []Candidate, targetPodNames []string) []Candidate {
	if len(targetPodNames) == 0 {
		return candidates
	}

	targetSet := make(map[string]bool, len(targetPodNames))
	for _, name := range targetPodNames {
		targetSet[name] = true
	}

	var result []Candidate
	for _, c := range candidates {
		if targetSet[c.PodName] {
			result = append(result, c)
		}
	}
	return result
}
