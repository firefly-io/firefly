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

package estimator

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	clusterinformers "github.com/karmada-io/karmada/pkg/generated/informers/externalversions/cluster/v1alpha1"
	clusterlisters "github.com/karmada-io/karmada/pkg/generated/listers/cluster/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	installinformers "github.com/carlory/firefly/pkg/generated/informers/externalversions/install/v1alpha1"
	installlisters "github.com/carlory/firefly/pkg/generated/listers/install/v1alpha1"
	"github.com/carlory/firefly/pkg/karmada/scheme"
)

const (
	// maxRetries is the number of times a cluster will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a cluster.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
	// name of the estimator controller finalizer
	EstimatorControllerFinalizerName = "estimator.karmada.install.firefly.io/finalizer"
)

// NewEstimatorController returns a new *Controller.
func NewEstimatorController(
	karmadaKubeClient clientset.Interface,
	karmadaClient karmadaversioned.Interface,
	clusterInformer clusterinformers.ClusterInformer,
	estimatorNamespace string,
	karmadaName string,
	fireflyKubeClient clientset.Interface,
	fireflyKarmadaInformer installinformers.KarmadaInformer,
) (*EstimatorController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "estimator-controller"})

	if karmadaKubeClient != nil && karmadaKubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("estimator_controller", karmadaKubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &EstimatorController{
		karmadaKubeClient:    karmadaKubeClient,
		karmadaClient:        karmadaClient,
		clustersLister:       clusterInformer.Lister(),
		clustersSynced:       clusterInformer.Informer().HasSynced,
		estimatorNamespace:   estimatorNamespace,
		karmadaName:          karmadaName,
		fireflyKubeClient:    fireflyKubeClient,
		fireflyKarmadaLister: fireflyKarmadaInformer.Lister(),
		fireflyKarmadaSynced: fireflyKarmadaInformer.Informer().HasSynced,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cluster"),
		workerLoopPeriod:     time.Second,
		eventBroadcaster:     broadcaster,
		eventRecorder:        recorder,
	}

	clusterInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addCluster,
		UpdateFunc: ctrl.updateCluster,
		DeleteFunc: ctrl.deleteCluster,
	})

	fireflyKarmadaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ctrl.syncKarmada,
	})

	return ctrl, nil
}

type EstimatorController struct {
	karmadaKubeClient clientset.Interface
	karmadaClient     karmadaversioned.Interface
	eventBroadcaster  record.EventBroadcaster
	eventRecorder     record.EventRecorder

	estimatorNamespace   string
	karmadaName          string
	fireflyKubeClient    clientset.Interface
	fireflyKarmadaLister installlisters.KarmadaLister
	fireflyKarmadaSynced cache.InformerSynced

	clustersLister clusterlisters.ClusterLister
	clustersSynced cache.InformerSynced

	// Cluster that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// cluster will be handled in parallel.
func (ctrl *EstimatorController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.karmadaKubeClient.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting estimator controller")
	defer klog.Infof("Shutting down estimator controller")

	if !cache.WaitForNamedCacheSync("estimator", ctx.Done(), ctrl.clustersSynced, ctrl.fireflyKarmadaSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, ctrl.worker, ctrl.workerLoopPeriod)
	}
	<-ctx.Done()
}

// worker runs a worker thread that just dequeues items, processes them, and
// marks them done. You may run as many of these in parallel as you wish; the
// workqueue guarantees that they will not end up processing the same service
// at the same time.
func (ctrl *EstimatorController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *EstimatorController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncEstimator(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *EstimatorController) addCluster(obj interface{}) {
	cluster := obj.(*clusterv1alpha1.Cluster)
	klog.V(4).InfoS("Adding cluster", "cluster", klog.KObj(cluster))
	ctrl.enqueue(cluster)
}

func (ctrl *EstimatorController) updateCluster(old, cur interface{}) {
	oldCluster := old.(*clusterv1alpha1.Cluster)
	curCluster := cur.(*clusterv1alpha1.Cluster)
	klog.V(4).InfoS("Updating cluster", "cluster", klog.KObj(oldCluster))
	ctrl.enqueue(curCluster)
}

func (ctrl *EstimatorController) deleteCluster(obj interface{}) {
	cluster, ok := obj.(*clusterv1alpha1.Cluster)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		cluster, ok = tombstone.Obj.(*clusterv1alpha1.Cluster)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Cluster %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting cluster", "cluster", klog.KObj(cluster))
	ctrl.enqueue(cluster)
}

func (ctrl *EstimatorController) syncKarmada(old, cur interface{}) {
	oldKarmada := old.(*installv1alpha1.Karmada)
	curKarmada := cur.(*installv1alpha1.Karmada)

	klog.V(4).InfoS("Sync karmada", "karmada", klog.KObj(oldKarmada))

	var needUpdate bool

	oldEstimator := oldKarmada.Spec.Scheduler.KarmadaSchedulerEstimator
	curEstimator := curKarmada.Spec.Scheduler.KarmadaSchedulerEstimator
	if !reflect.DeepEqual(oldEstimator, curEstimator) ||
		(oldEstimator.ImageTag == "" && oldKarmada.Spec.KarmadaVersion != curKarmada.Spec.KarmadaVersion) {
		needUpdate = true
	}

	oldSchedulerArgs := oldKarmada.Spec.Scheduler.KarmadaScheduler.ExtraArgs
	curSchedulerArgs := curKarmada.Spec.Scheduler.KarmadaScheduler.ExtraArgs
	if !reflect.DeepEqual(oldSchedulerArgs, curSchedulerArgs) {
		oldEstimatorArg := oldSchedulerArgs["disable-scheduler-estimator-in-pull-mode"]
		curEstimatorArg := curSchedulerArgs["disable-scheduler-estimator-in-pull-mode"]
		if oldEstimatorArg != curEstimatorArg {
			needUpdate = true
		}
	}

	if !needUpdate {
		return
	}

	clusters, err := ctrl.clustersLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("Failed to list clusters: %v", err)
	}
	for _, cluster := range clusters {
		ctrl.enqueue(cluster)
	}
}

func (ctrl *EstimatorController) enqueue(cluster *clusterv1alpha1.Cluster) {
	ctrl.queue.Add(cluster.Name)
}

func (ctrl *EstimatorController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, v1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing estimator, retrying", "cluster", klog.KRef("", key.(string)), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping cluster out of the queue", "cluster", klog.KRef("", key.(string)), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *EstimatorController) syncEstimator(ctx context.Context, key string) error {
	startTime := time.Now()
	klog.V(4).InfoS("Started syncing estimator", "cluster", klog.KRef("", key), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing estimator", "cluster", klog.KRef("", key), "duration", time.Since(startTime))
	}()

	cluster, err := ctrl.clustersLister.Get(key)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Cluster has been deleted", "cluster", klog.KRef("", key))
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	cluster = cluster.DeepCopy()
	// examine DeletionTimestamp to determine if object is under deletion
	if cluster.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(cluster, EstimatorControllerFinalizerName) {
			controllerutil.AddFinalizer(cluster, EstimatorControllerFinalizerName)
			cluster, err = ctrl.karmadaClient.ClusterV1alpha1().Clusters().Update(ctx, cluster, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(cluster, EstimatorControllerFinalizerName) {
			// our finalizer is present, so lets remove the orpand estimator
			karmada, err := ctrl.fireflyKarmadaLister.Karmadas(ctrl.estimatorNamespace).Get(ctrl.karmadaName)
			if err != nil {
				if errors.IsNotFound(err) {
					klog.V(2).InfoS("Karmada has been deleted, the estimator will be deleted by the gc controller of host cluster",
						"karmada", klog.KRef(ctrl.estimatorNamespace, ctrl.karmadaName),
						"cluster", cluster.Name)
					return nil
				}
				klog.ErrorS(err, "failed to get karmada", "karmada", ctrl.karmadaName)
				return err
			}
			if err := ctrl.RemoveEstimator(ctx, karmada, cluster); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				klog.ErrorS(err, "failed to remove estimator", "karmada", ctrl.karmadaName, "cluster", cluster.Name)
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(cluster, EstimatorControllerFinalizerName)
			// Stop reconciliation as the item is being deleted
			_, err = ctrl.karmadaClient.ClusterV1alpha1().Clusters().Update(ctx, cluster, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			return nil
		}
	}

	karmada, err := ctrl.fireflyKarmadaLister.Karmadas(ctrl.estimatorNamespace).Get(ctrl.karmadaName)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(2).InfoS("Karmada has been deleted", "karmada", klog.KRef(ctrl.estimatorNamespace, ctrl.karmadaName))
			return nil
		}
		return err
	}

	if karmada.DeletionTimestamp != nil {
		klog.V(2).InfoS("Karmada is terminating", "karmada", klog.KRef(ctrl.estimatorNamespace, ctrl.karmadaName))
		return nil
	}

	if cluster.Spec.SyncMode == clusterv1alpha1.Pull {
		schedulerArgs := karmada.Spec.Scheduler.KarmadaScheduler.ExtraArgs
		disableEstimatorVal, ok := schedulerArgs["disable-scheduler-estimator-in-pull-mode"]
		if ok {
			disableEstimator, err := strconv.ParseBool(disableEstimatorVal)
			if err != nil {
				return err
			}
			if disableEstimator {
				return nil
			}
		}
	}

	klog.InfoS("Syncing estimator", "cluster", cluster.Name)
	return ctrl.EnsureEstimator(ctx, karmada, cluster)
}

func (ctrl *EstimatorController) EnsureEstimator(ctx context.Context, karmada *installv1alpha1.Karmada, cluster *clusterv1alpha1.Cluster) error {
	if err := ctrl.EnsureEstimatorKubeconfigSecret(ctx, karmada, cluster); err != nil {
		return err
	}

	if err := ctrl.EnsureEstimatorService(ctx, karmada, cluster); err != nil {
		return err
	}

	if err := ctrl.EnsureEstimatorDeployment(ctx, karmada, cluster); err != nil {
		return err
	}
	return nil
}

func (ctrl *EstimatorController) RemoveEstimator(ctx context.Context, karmada *installv1alpha1.Karmada, cluster *clusterv1alpha1.Cluster) error {
	estimatorName := GenerateEstimatorName(karmada.Name, defaultEstimatorServicePrefix, cluster.Name)
	err := ctrl.fireflyKubeClient.CoreV1().Services(karmada.Namespace).Delete(ctx, estimatorName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	err = ctrl.fireflyKubeClient.AppsV1().Deployments(karmada.Namespace).Delete(ctx, estimatorName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	secretName := GenerateEstimatorKubeConfigSecretName(karmada.Name, defaultEstimatorServicePrefix, cluster.Name)
	err = ctrl.fireflyKubeClient.CoreV1().Secrets(karmada.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	return client.IgnoreNotFound(err)
}
