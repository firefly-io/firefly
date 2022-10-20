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

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
)

func GenerateDatabaseSecretName(clusterpedia *installv1alpha1.Clusterpedia) string {
	componentName := constants.ClusterpediaComponentInternalStoragePostgres
	return fmt.Sprintf("%s-internalstorage-password", componentName)
}

func GenerateDatabaseConfigMapName(clusterpedia *installv1alpha1.Clusterpedia) string {
	return constants.ClusterpediaComponentInternalStorage
}
