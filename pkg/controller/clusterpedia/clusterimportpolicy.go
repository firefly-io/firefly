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

package clusterpedia

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

var gvr = schema.GroupVersionResource{Group: "policy.clusterpedia.io", Version: "v1alpha1", Resource: "clusterimportpolicies"}

func (ctrl *ClusterpediaController) EnsureClusterImportPolicy(clusterpedia *installv1alpha1.Clusterpedia) error {
	exists, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var tmpl string
	provider := clusterpedia.Spec.ControlplaneProvider
	switch {
	case provider.Karmada != nil:
		tmpl = KarmadaClusterImportPolicyTemplate
	default:
		return fmt.Errorf("unsupported controlplane provider")
	}

	return ctrl.applyPolicy(clusterpedia, tmpl)
}

func (ctrl *ClusterpediaController) applyPolicy(clusterpedia *installv1alpha1.Clusterpedia, tmpl string) error {
	data, err := yaml.Marshal(tmpl)
	if err != nil {
		return err
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(data); err != nil {
		return err
	}

	client, err := ctrl.GetControlplaneDynamicClientFromProvider(clusterpedia)
	if err != nil {
		return err
	}

	_, err = client.Resource(gvr).Create(context.TODO(), obj, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		old, err := client.Resource(gvr).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.SetResourceVersion(old.GetResourceVersion())
		_, err = client.Resource(gvr).Update(context.TODO(), obj, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
