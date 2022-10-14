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

package karmada

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/resource"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	utilresource "github.com/carlory/firefly/pkg/util/resource"
)

func (ctrl *KarmadaController) EnsureKarmadaCRDs(karmada *installv1alpha1.Karmada) error {
	clientConfig, err := ctrl.GenerateClientConfig(karmada)
	if err != nil {
		return err
	}
	result := utilresource.NewBuilder(clientConfig).
		Unstructured().
		FilenameParam(false, &resource.FilenameOptions{Recursive: false, Filenames: []string{"./pkg/controller/karmada/crds"}}).
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
