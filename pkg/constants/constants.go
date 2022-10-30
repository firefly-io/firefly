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

package constants

import "time"

const (
	// APICallRetryInterval defines how long firefly should wait before retrying a failed API operation
	APICallRetryInterval = 500 * time.Millisecond

	// KarmadaSystemNamespace defines the leader selection namespace for karmada components
	KarmadaSystemNamespace = "karmada-system"
	// KarmadaComponentEtcd defines the name of the built-in etcd cluster component
	KarmadaComponentEtcd = "etcd"
	// KarmadaComponentKubeAPIServer defines the name of the karmada-apiserver component
	KarmadaComponentKubeAPIServer = "karmada-apiserver"
	// KarmadaComponentAggregratedAPIServer defines the name of the karmada-aggregated-apiserver component
	KarmadaComponentAggregratedAPIServer = "karmada-aggregated-apiserver"
	// KarmadaComponentKubeControllerManager defines the name of the karmada-kube-controller-manager component
	KarmadaComponentKubeControllerManager = "karmada-kube-controller-manager"
	// KarmadaComponentScheduler defines the name of the karmada-scheduler component
	KarmadaComponentScheduler = "karmada-scheduler"
	// KarmadaComponentDescheduler defines the name of the karmada-descheduler component
	KarmadaComponentDescheduler = "karmada-descheduler"
	// KarmadaComponentControllerManager defines the name of the karmada-controller-manager component
	KarmadaComponentControllerManager = "karmada-controller-manager"
	// KarmadaComponentWebhook defines the name of the karmada-webhook component
	KarmadaComponentWebhook = "karmada-webhook"
	// KarmadaComponentSchedulerEstimator defines the name of the karmada-scheduler-estimator component
	KarmadaComponentSchedulerEstimator = "karmada-scheduler-estimator"
	// FireflyComponentKarmadaManager defines the name of the karmada-karmada-manager component
	FireflyComponentKarmadaManager = "firefly-karmada-manager"
	// FireflyComponentKarmadaWebhook defines the name of the karmada-karmada-webhook component
	FireflyComponentKarmadaWebhook = "firefly-karmada-webhook"

	// ClusterpediaSystemNamespace defines the leader selection namespace for clusterpedia components
	ClusterpediaSystemNamespace = "clusterpedia-system"
	// ClusterpediaComponentAPIServer defines the name of the clusterpedia-apiserver component
	ClusterpediaComponentAPIServer = "clusterpedia-apiserver"
	// ClusterpediaComponentControllerManager defines the name of the clusterpedia-controller-manager component
	ClusterpediaComponentControllerManager = "clusterpedia-controller-manager"
	// ClusterpediaComponentClusterSynchroManager defines the name of the clusterpedia-clustersynchro-manager component
	ClusterpediaComponentClusterSynchroManager = "clusterpedia-clustersynchro-manager"
	// ClusterpediaComponentInternalStorage defines the name of the clusterpedia-internalstorage component
	ClusterpediaComponentInternalStorage = "clusterpedia-internalstorage"
	// ClusterpediaComponentInternalStoragePostgres defines the name of the clusterpedia-internalstorage-postgres component
	ClusterpediaComponentInternalStoragePostgres = "clusterpedia-internalstorage-postgres"
	// ClusterpediaComponentInternalStorageMySQL defines the name of the clusterpedia-internalstorage-mysql component
	ClusterpediaComponentInternalStorageMySQL = "clusterpedia-internalstorage-mysql"
)
