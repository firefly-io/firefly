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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	fireflyclient "github.com/carlory/firefly/pkg/generated/clientset/versioned"
	installinformers "github.com/carlory/firefly/pkg/generated/informers/externalversions/install/v1alpha1"
	installlisters "github.com/carlory/firefly/pkg/generated/listers/install/v1alpha1"
)

const (
	// maxRetries is the number of times a clusterpedia will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a clusterpedia.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15

	// name of the clusterpedia controller finalizer
	ClusterpediaControllerFinalizerName = "clusterpedia.install.firefly.io/finalizer"
)

// NewClusterpediaController returns a new *Controller.
func NewClusterpediaController(
	client clientset.Interface,
	fireflyClient fireflyclient.Interface,
	clusterpediaInformer installinformers.ClusterpediaInformer) (*ClusterpediaController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "clusterpedia-controller"})

	if client != nil && client.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("clusterpedia_controller", client.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &ClusterpediaController{
		client:              client,
		fireflyClient:       fireflyClient,
		clusterpediasLister: clusterpediaInformer.Lister(),
		clusterpediasSynced: clusterpediaInformer.Informer().HasSynced,
		queue:               workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "clusterpedia"),
		workerLoopPeriod:    time.Second,
		eventBroadcaster:    broadcaster,
		eventRecorder:       recorder,
	}

	clusterpediaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addClusterpedia,
		UpdateFunc: ctrl.updateClusterpedia,
		DeleteFunc: ctrl.deleteClusterpedia,
	})

	return ctrl, nil
}

type ClusterpediaController struct {
	client           clientset.Interface
	fireflyClient    fireflyclient.Interface
	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	clusterpediasLister installlisters.ClusterpediaLister
	clusterpediasSynced cache.InformerSynced

	// Clusterpedia that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// clusterpedia will be handled in parallel.
func (ctrl *ClusterpediaController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.client.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting clusterpedia controller")
	defer klog.Infof("Shutting down clusterpedia controller")

	if !cache.WaitForNamedCacheSync("clusterpedia", ctx.Done(), ctrl.clusterpediasSynced) {
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
func (ctrl *ClusterpediaController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *ClusterpediaController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncClusterpedia(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *ClusterpediaController) addClusterpedia(obj interface{}) {
	clusterpedia := obj.(*installv1alpha1.Clusterpedia)
	klog.V(4).InfoS("Adding clusterpedia", "clusterpedia", klog.KObj(clusterpedia))
	ctrl.enqueue(clusterpedia)
}

func (ctrl *ClusterpediaController) updateClusterpedia(old, cur interface{}) {
	oldClusterpedia := old.(*installv1alpha1.Clusterpedia)
	curClusterpedia := cur.(*installv1alpha1.Clusterpedia)
	klog.V(4).InfoS("Updating clusterpedia", "clusterpedia", klog.KObj(oldClusterpedia))
	ctrl.enqueue(curClusterpedia)
}

func (ctrl *ClusterpediaController) deleteClusterpedia(obj interface{}) {
	clusterpedia, ok := obj.(*installv1alpha1.Clusterpedia)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		clusterpedia, ok = tombstone.Obj.(*installv1alpha1.Clusterpedia)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Clusterpedia %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting clusterpedia", "clusterpedia", klog.KObj(clusterpedia))
	ctrl.enqueue(clusterpedia)
}

func (ctrl *ClusterpediaController) enqueue(clusterpedia *installv1alpha1.Clusterpedia) {
	key, err := cache.MetaNamespaceKeyFunc(clusterpedia)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *ClusterpediaController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	ns, name, keyErr := cache.SplitMetaNamespaceKey(key.(string))
	if keyErr != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing clusterpedia, retrying", "clusterpedia", klog.KRef(ns, name), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping clusterpedia out of the queue", "clusterpedia", klog.KRef(ns, name), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *ClusterpediaController) syncClusterpedia(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing clusterpedia", "clusterpedia", klog.KRef(namespace, name), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing clusterpedia", "clusterpedia", klog.KRef(namespace, name), "duration", time.Since(startTime))
	}()

	clusterpedia, err := ctrl.clusterpediasLister.Clusterpedias(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Clusterpedia has been deleted", "clusterpedia", klog.KRef(namespace, name))
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	clusterpedia = clusterpedia.DeepCopy()

	// examine DeletionTimestamp to determine if object is under deletion
	if clusterpedia.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(clusterpedia, ClusterpediaControllerFinalizerName) {
			controllerutil.AddFinalizer(clusterpedia, ClusterpediaControllerFinalizerName)
			clusterpedia, err = ctrl.fireflyClient.InstallV1alpha1().Clusterpedias(clusterpedia.Namespace).Update(ctx, clusterpedia, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(clusterpedia, ClusterpediaControllerFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := ctrl.deleteUnableGCResources(clusterpedia); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(clusterpedia, ClusterpediaControllerFinalizerName)
			_, err := ctrl.fireflyClient.InstallV1alpha1().Clusterpedias(clusterpedia.Namespace).Update(ctx, clusterpedia, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			// Stop reconciliation as the item is being deleted
			return nil
		}
	}

	klog.InfoS("Syncing clusterpedia", "clusterpedia", klog.KObj(clusterpedia))

	if err := ctrl.EnsureNamespace(clusterpedia); err != nil {
		return err
	}

	if err := ctrl.EnsureClusterpediaCRDs(clusterpedia); err != nil {
		return err
	}

	if err := ctrl.EnsureInternalStorage(clusterpedia); err != nil {
		return err
	}

	if err := ctrl.EnsureAPIServer(clusterpedia); err != nil {
		return err
	}

	if err := ctrl.EnsureControllerManager(clusterpedia); err != nil {
		return err
	}

	if err := ctrl.EnsureClusterSynchroManager(clusterpedia); err != nil {
		return err
	}

	return nil
}

func (ctrl *ClusterpediaController) EnsureNamespace(clusterpedia *installv1alpha1.Clusterpedia) error {
	kubeClient, err := ctrl.GetControlplaneClient(clusterpedia)
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterpedia.Namespace}}
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return err
	}
	if hasProvider {
		ns.Name = constants.ClusterpediaSystemNamespace
	}
	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (ctrl *ClusterpediaController) deleteUnableGCResources(clusterpedia *installv1alpha1.Clusterpedia) error {
	err := ctrl.RemoveClusterpediaCRDs(clusterpedia)
	if err != nil {
		return err
	}
	err = ctrl.RemoveClusterpediaAPIService(clusterpedia)
	if err != nil {
		return err
	}
	return ctrl.removeNamespaceFromControlplaneProvider(clusterpedia)
}

func (ctrl *ClusterpediaController) removeNamespaceFromControlplaneProvider(clusterpedia *installv1alpha1.Clusterpedia) error {
	hasProvider, err := ctrl.IsControllPlaneProviderExists(clusterpedia)
	if err != nil {
		return err
	}

	// if no provider, do nothing
	if !hasProvider {
		return nil
	}

	kubeClient, err := ctrl.GetControlplaneClientFromProvider(clusterpedia)
	if err != nil {
		return err
	}

	err = kubeClient.CoreV1().Namespaces().Delete(context.TODO(), constants.ClusterpediaSystemNamespace, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}
