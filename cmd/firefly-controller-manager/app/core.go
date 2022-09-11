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

package app

import (
	"context"
	"fmt"

	"k8s.io/controller-manager/controller"

	"github.com/carlory/firefly/pkg/controller/karmada"
)

func startKarmadaController(ctx context.Context, controllerContext ControllerContext) (controller.Interface, bool, error) {
	ctrl, err := karmada.NewKarmadaController(
		controllerContext.ClientBuilder.ClientOrDie("firefly-karmada-controller"),
		controllerContext.FireflyInformerFactory.Install().V1alpha1().Karmadas(),
	)
	if err != nil {
		return nil, true, fmt.Errorf("failed to start the pvc karmada controller: %v", err)
	}
	go ctrl.Run(ctx, 1)
	return nil, true, nil
}
