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

package v1alpha1

import (
	utilpointer "k8s.io/utils/pointer"
)

func SetDefaults_Clusterpedia(obj *Clusterpedia) {
	if obj.Spec.ImageRepository == "" {
		obj.Spec.ImageRepository = "ghcr.io/clusterpedia-io/clusterpedia"
	}

	if obj.Spec.Version == "" {
		obj.Spec.Version = "latest"
	}

	storage := &obj.Spec.Storage
	if storage.Postgres == nil && storage.MySQL == nil {
		// choose internal postgres as the default storage
		storage.Postgres = &Postgres{
			Local: &LocalPostgres{
				ImageMeta: ImageMeta{
					ImageRepository: "docker.io/library",
					ImageName:       "postgres",
					ImageTag:        "12",
				},
			},
		}
	} else if storage.Postgres != nil && storage.Postgres.Local != nil {
		local := storage.Postgres.Local
		if local.ImageMeta.ImageRepository == "" {
			local.ImageMeta.ImageRepository = "docker.io/library"
		}
		if local.ImageMeta.ImageName == "" {
			local.ImageMeta.ImageName = "postgres"
		}
		if local.ImageMeta.ImageTag == "" {
			local.ImageMeta.ImageTag = "12"
		}
	} else if storage.MySQL != nil && storage.MySQL.Local != nil {
		local := storage.MySQL.Local
		if local.ImageMeta.ImageRepository == "" {
			local.ImageMeta.ImageRepository = "docker.io/library"
		}
		if local.ImageMeta.ImageName == "" {
			local.ImageMeta.ImageName = "mysql"
		}
		if local.ImageMeta.ImageTag == "" {
			local.ImageMeta.ImageTag = "8"
		}
	}

	apiServer := &obj.Spec.APIServer
	if apiServer.Replicas == nil {
		apiServer.Replicas = utilpointer.Int32(1)
	}

	controllerManager := &obj.Spec.ControllerManager
	if controllerManager.Replicas == nil {
		controllerManager.Replicas = utilpointer.Int32(1)
	}

	synchro := &obj.Spec.ClusterpediaSynchroManager
	if synchro.Replicas == nil {
		synchro.Replicas = utilpointer.Int32(1)
	}
}
