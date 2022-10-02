/*
Copyright 2022 The Firefly Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
)

// MergeBoolMaps merges two bool maps.
func MergeBoolMaps(maps ...map[string]bool) map[string]bool {
	result := map[string]bool{}
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

// MergeStringMaps merges two string maps.
func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

// ConvertToCommandOrArgs converts a string map to a command or args array.
func ConvertToCommandOrArgs(m map[string]string) []string {
	keys := sets.NewString()
	for k := range m {
		keys.Insert(k)
	}
	array := make([]string, 0, len(keys))
	for _, v := range keys.List() {
		array = append(array, fmt.Sprintf("--%s=%s", v, m[v]))
	}
	return array
}
