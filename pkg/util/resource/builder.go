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

package resource

import (
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NewBuilder is a helper function that accepts a rest.Config as an input parameter and
// returns a resource.Builder instance.
func NewBuilder(restConfig *rest.Config) *resource.Builder {
	return resource.NewBuilder(clientGetter{restConfig: restConfig})
}

// NewBuilder is a helper function that accepts a kubeconfig secret as an input parameter and
// returns a resource.Builder instance.
func NewBuilderFromKubeConfigSecret(client kubernetes.Interface, namespace, secretName, userAgent string) (*resource.Builder, error) {
	clientConfig, err := GetClientConfigFromKubeConfigSecret(client, namespace, secretName, userAgent)
	if err != nil {
		return nil, err
	}
	return NewBuilder(clientConfig), nil
}
