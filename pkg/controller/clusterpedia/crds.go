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
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/resource"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
)

const (
	// the user-agent name is used when talking to kube-apiserver
	userAgentName = "clusterpedia-controller"
)

func (ctrl *ClusterpediaController) EnsureClusterpediaCRDs(clusterpedia *installv1alpha1.Clusterpedia) error {
	builder, err := ctrl.NewResourceBuilder(clusterpedia)
	if err != nil {
		return err
	}

	result := builder.Unstructured().
		FilenameParam(false, &resource.FilenameOptions{Recursive: false, Filenames: []string{"./pkg/controller/clusterpedia/crds"}}).
		Flatten().Do()

	return result.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		_, err1 := resource.NewHelper(info.Client, info.Mapping).Create(info.Namespace, true, info.Object)
		if err1 != nil {
			if !errors.IsAlreadyExists(err1) {
				return err1
			}
		}
		return nil
	})
}

func (ctrl *ClusterpediaController) RemoveClusterpediaCRDs(clusterpedia *installv1alpha1.Clusterpedia) error {
	builder, err := ctrl.NewResourceBuilder(clusterpedia)
	if err != nil {
		return err
	}

	result := builder.Unstructured().
		FilenameParam(false, &resource.FilenameOptions{Recursive: false, Filenames: []string{"./pkg/controller/clusterpedia/crds"}}).
		Flatten().Do()

	return result.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		_, err1 := resource.NewHelper(info.Client, info.Mapping).Delete(info.Namespace, info.Name)
		if err1 != nil {
			if !errors.IsNotFound(err1) {
				return err1
			}
		}
		return nil
	})
}

func (ctrl *ClusterpediaController) NewResourceBuilder(clusterpedia *installv1alpha1.Clusterpedia) (*resource.Builder, error) {
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return nil, err
	}

	if !hasProvider {
		return nil, fmt.Errorf("unsupported without provider")
	}

	kubeconfigSecretName, err := ctrl.KubeConfigSecretNameFromProvider(clusterpedia)
	if err != nil {
		return nil, err
	}
	return utilresource.NewBuilderFromKubeConfigSecret(ctrl.client, clusterpedia.Namespace, kubeconfigSecretName, userAgentName)
}
