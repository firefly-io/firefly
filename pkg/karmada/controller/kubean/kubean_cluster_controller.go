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

package kubean

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/carlory/firefly/pkg/scheme"
)

const (
	// maxRetries is the number of times a cluster will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a cluster.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15

	// name of the cluster controller finalizer
	ClusterControllerFinalizerName = "cluster.kubean.firefly.io/finalizer"
)

var (
	clusterGVR = schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "clusters"}
)

// NewClusterController returns a new *Controller.
func NewClusterController(
	karmadaNamespace string,
	client clientset.Interface,
	dynamicClient dynamic.Interface,
	clustersInformer informers.GenericInformer,
	fireflyDynamicClient dynamic.Interface,
	hostClustersInformer informers.GenericInformer) (*ClusterController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "kubean-cluster-controller"})

	if client != nil && client.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("cluster_controller", client.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &ClusterController{
		karmadaNamespace:   karmadaNamespace,
		client:             client,
		clusterClient:      dynamicClient.Resource(clusterGVR),
		clustersLister:     clustersInformer.Lister(),
		clustersSynced:     clustersInformer.Informer().HasSynced,
		hostClusterClient:  fireflyDynamicClient.Resource(clusterGVR),
		hostClustersLister: hostClustersInformer.Lister(),
		hostClustersSynced: hostClustersInformer.Informer().HasSynced,
		queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cluster"),
		workerLoopPeriod:   time.Second,
		eventBroadcaster:   broadcaster,
		eventRecorder:      recorder,
	}

	clustersInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addCluster,
		UpdateFunc: ctrl.updateCluster,
		DeleteFunc: ctrl.deleteCluster,
	})

	hostClustersInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			hostCluster := obj.(*unstructured.Unstructured)
			cluster := convert_kubean_cluster_host_to_karmada(hostCluster)
			ctrl.updateStatus(cluster)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			hostCluster := newObj.(*unstructured.Unstructured)
			cluster := convert_kubean_cluster_host_to_karmada(hostCluster)
			ctrl.updateStatus(cluster)
		},
		DeleteFunc: func(obj interface{}) {
			hostCluster, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			cluster := convert_kubean_cluster_host_to_karmada(hostCluster)
			ctrl.updateStatus(cluster)
		},
	})

	return ctrl, nil
}

type ClusterController struct {
	client clientset.Interface

	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	clusterClient  dynamic.NamespaceableResourceInterface
	clustersLister cache.GenericLister
	clustersSynced cache.InformerSynced

	hostClusterClient  dynamic.NamespaceableResourceInterface
	hostClustersLister cache.GenericLister
	hostClustersSynced cache.InformerSynced

	karmadaNamespace string

	// Kubean that need to be updated. A channel is inappropriate here,
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
func (ctrl *ClusterController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.client.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting kubean cluster controller")
	defer klog.Infof("Shutting down kubean cluster controller")

	if !cache.WaitForNamedCacheSync("cluster", ctx.Done(), ctrl.clustersSynced, ctrl.hostClustersSynced) {
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
func (ctrl *ClusterController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *ClusterController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncCluster(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *ClusterController) addCluster(obj interface{}) {
	cluster := obj.(*unstructured.Unstructured)
	klog.V(4).InfoS("Adding cluster", "cluster", klog.KObj(cluster))
	ctrl.enqueue(cluster)
}

func (ctrl *ClusterController) updateCluster(old, cur interface{}) {
	oldCluster := old.(*unstructured.Unstructured)
	curCluster := cur.(*unstructured.Unstructured)
	klog.V(4).InfoS("Updating cluster", "cluster", klog.KObj(oldCluster))
	ctrl.enqueue(curCluster)
}

func (ctrl *ClusterController) deleteCluster(obj interface{}) {
	cluster, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		cluster, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Cluster %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting cluster", "cluster", klog.KObj(cluster))
	ctrl.enqueue(cluster)
}

func (ctrl *ClusterController) enqueue(cluster *unstructured.Unstructured) {
	key, err := cache.MetaNamespaceKeyFunc(cluster)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *ClusterController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	ns, name, keyErr := cache.SplitMetaNamespaceKey(key.(string))
	if keyErr != nil {
		klog.ErrorS(err, "Failed to split meta cache key", "cacheKey", key)
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing cluster, retrying", "cluster", klog.KRef(ns, name), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping cluster out of the queue", "cluster", klog.KRef(ns, name), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *ClusterController) syncCluster(ctx context.Context, key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing cluster", "cluster", name, "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing cluster", "cluster", name, "duration", time.Since(startTime))
	}()

	clusterObj, err := ctrl.clustersLister.Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Cluster has been deleted", "cluster", name)
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	cluster := clusterObj.DeepCopyObject().(*unstructured.Unstructured)

	// examine DeletionTimestamp to determine if object is under deletion
	if cluster.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(cluster, ClusterControllerFinalizerName) {
			controllerutil.AddFinalizer(cluster, ClusterControllerFinalizerName)
			cluster, err = ctrl.clusterClient.Update(ctx, cluster, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(cluster, ClusterControllerFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := ctrl.deleteUnableGCResources(ctx, cluster); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(cluster, ClusterControllerFinalizerName)
			_, err := ctrl.clusterClient.Update(ctx, cluster, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			// Stop reconciliation as the item is being deleted
			return nil
		}
	}

	klog.InfoS("Syncing cluster", "cluster", klog.KObj(cluster))

	hostCluster := convert_kubean_cluster_karmada_to_host(ctrl.karmadaNamespace, cluster)
	klog.InfoS("convert kubean cluster from karmada to host", "karmada", klog.KObj(cluster), "host", klog.KObj(hostCluster))
	dropInvaildFields(hostCluster)

	existing, err := ctrl.hostClusterClient.Get(ctx, hostCluster.GetName(), metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get object from host", "cluster", hostCluster.GetName())
			return err
		}
		klog.InfoS("Creating cluster on the host cluster", "cluster", klog.KObj(hostCluster))
		_, err := ctrl.hostClusterClient.Create(ctx, hostCluster, metav1.CreateOptions{})
		if err != nil {
			klog.ErrorS(err, "failed to create object on the host", "cluster", hostCluster.GetName())
			return err
		}
		return nil
	}

	hostCluster.SetUID(existing.GetUID())
	hostCluster.SetResourceVersion(existing.GetResourceVersion())
	_, err = ctrl.hostClusterClient.Update(ctx, hostCluster, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to update object on the host", "cluster", hostCluster.GetName())
	}
	return err
}

func (ctrl *ClusterController) updateStatus(cluster *unstructured.Unstructured) error {
	existing, err := ctrl.clusterClient.Get(context.TODO(), cluster.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	toUpdate := cluster.DeepCopy()
	dropInvaildFields(toUpdate)
	toUpdate.SetUID(existing.GetUID())
	toUpdate.SetResourceVersion(existing.GetResourceVersion())
	_, err = ctrl.clusterClient.UpdateStatus(context.TODO(), toUpdate, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to update status", "cluster", existing.GetName())
	}
	return err
}

func (ctrl *ClusterController) deleteUnableGCResources(ctx context.Context, cluster *unstructured.Unstructured) error {
	hostCluster := convert_kubean_cluster_karmada_to_host(ctrl.karmadaNamespace, cluster)
	err := ctrl.hostClusterClient.Delete(ctx, hostCluster.GetName(), metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}
