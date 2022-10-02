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
	"context"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
)

func KubeConfigFromSecret(client kubernetes.Interface, cluster *clusterv1alpha1.Cluster) (*clientcmdapi.Config, error) {

	client.CoreV1().Secrets(cluster.Spec.SecretRef.Namespace).Get(context.TODO(), cluster.Spec.SecretRef.Name, metav1.GetOptions{})
	cfg := &clientcmdapi.Config{
		Clusters: []clientcmdapi.NamedCluster{
			{
				Name: cluster.Name,
				Cluster: clientcmdapi.Cluster{
					Server:                cluster.Spec.APIEndpoint,
					InsecureSkipTLSVerify: cluster.Spec.InsecureSkipTLSVerification,
				},
			},
		},
		AuthInfos: []clientcmdapi.NamedAuthInfo{
			{
				Name:     cluster.Name,
				AuthInfo: clientcmdapi.AuthInfo{},
			},
		},
	}
	return cfg, nil
}
