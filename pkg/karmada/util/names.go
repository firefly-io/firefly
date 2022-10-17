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
	"hash/fnv"
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"
)

// ExecutionSpacePrefix is the prefix of execution space
const ExecutionSpacePrefix = "karmada-es-"

// GenerateExecutionSpaceName generates execution space name for the given member cluster
func GenerateExecutionSpaceName(clusterName string) (string, error) {
	if clusterName == "" {
		return "", fmt.Errorf("the member cluster name is empty")
	}
	executionSpace := ExecutionSpacePrefix + clusterName
	return executionSpace, nil
}

// GenerateWorkName will generate work name by its name and the hash of its namespace, kind and name.
func GenerateWorkName(kind, name, namespace string) string {
	// The name of resources, like 'Role'/'ClusterRole'/'RoleBinding'/'ClusterRoleBinding',
	// may contain symbols(like ':') that are not allowed by CRD resources which require the
	// name can be used as a DNS subdomain name. So, we need to replace it.
	// These resources may also allow for other characters(like '&','$') that are not allowed
	// by CRD resources, we only handle the most common ones now for performance concerns.
	// For more information about the DNS subdomain name, please refer to
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names.
	if strings.Contains(name, ":") {
		name = strings.ReplaceAll(name, ":", ".")
	}

	var workName string
	if len(namespace) == 0 {
		workName = strings.ToLower(name + "-" + kind)
	} else {
		workName = strings.ToLower(namespace + "-" + name + "-" + kind)
	}
	hash := fnv.New32a()
	DeepHashObject(hash, workName)
	return fmt.Sprintf("%s-%s", name, rand.SafeEncodeString(fmt.Sprint(hash.Sum32())))
}
