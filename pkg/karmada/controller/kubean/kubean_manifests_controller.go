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
	"k8s.io/apimachinery/pkg/labels"
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

	"github.com/carlory/firefly/pkg/karmada/scheme"
)

var (
	manifestGVR = schema.GroupVersionResource{Group: "kubean.io", Version: "v1alpha1", Resource: "manifests"}
)

// NewManifestController returns a new *Controller.
func NewManifestController(
	kubeClient clientset.Interface,
	dynamicClient dynamic.Interface,
	manifestInformer informers.GenericInformer,
	hostManifestInformer informers.GenericInformer,
) (*ManifestController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "manifest-controller"})

	if kubeClient != nil && kubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("manifest_controller", kubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &ManifestController{
		kubeClient:         kubeClient,
		manifestClient:     dynamicClient.Resource(manifestGVR),
		manifestLister:     manifestInformer.Lister(),
		manifestSynced:     manifestInformer.Informer().HasSynced,
		hostManifestLister: hostManifestInformer.Lister(),
		hostManifestSynced: hostManifestInformer.Informer().HasSynced,
		queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "manifest"),
		workerLoopPeriod:   time.Second,
		eventBroadcaster:   broadcaster,
		eventRecorder:      recorder,
	}
	manifestInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addManifest,
		UpdateFunc: ctrl.updateManifest,
		DeleteFunc: ctrl.deleteManifest,
	})
	hostManifestInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addManifest,
		UpdateFunc: ctrl.updateManifest,
		DeleteFunc: ctrl.deleteManifest,
	})
	return ctrl, nil
}

type ManifestController struct {
	kubeClient       clientset.Interface
	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	manifestClient dynamic.NamespaceableResourceInterface
	manifestLister cache.GenericLister
	manifestSynced cache.InformerSynced

	hostManifestLister cache.GenericLister
	hostManifestSynced cache.InformerSynced

	// Manifest that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// manifest will be handled in parallel.
func (ctrl *ManifestController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.kubeClient.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting kubean manifest controller")
	defer klog.Infof("Shutting down kubean manifest controller")

	if !cache.WaitForNamedCacheSync("manifest", ctx.Done(), ctrl.manifestSynced, ctrl.hostManifestSynced) {
		return
	}

	manifests, _ := ctrl.manifestLister.List(labels.Everything())
	for _, manifest := range manifests {
		ctrl.addManifest(manifest)
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
func (ctrl *ManifestController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *ManifestController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncManifest(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *ManifestController) addManifest(obj interface{}) {
	manifest := obj.(*unstructured.Unstructured)
	klog.V(4).InfoS("Adding manifest", "manifest", klog.KObj(manifest))
	ctrl.enqueue(manifest)
}

func (ctrl *ManifestController) updateManifest(old, cur interface{}) {
	oldManifest := old.(*unstructured.Unstructured)
	curManifest := cur.(*unstructured.Unstructured)
	klog.V(4).InfoS("Updating manifest", "manifest", klog.KObj(oldManifest))
	ctrl.enqueue(curManifest)
}

func (ctrl *ManifestController) deleteManifest(obj interface{}) {
	manifest, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		manifest, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Manifest %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting manifest", "manifest", klog.KObj(manifest))
	ctrl.enqueue(manifest)
}

func (ctrl *ManifestController) enqueue(manifest *unstructured.Unstructured) {
	key, err := cache.MetaNamespaceKeyFunc(manifest)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *ManifestController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing manifest, retrying", "manifest", klog.KRef("", key.(string)), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping manifest out of the queue", "manifest", klog.KRef("", key.(string)), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *ManifestController) syncManifest(ctx context.Context, key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing manifest", "manifest", klog.KRef("", key), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing manifest", "manifest", klog.KRef("", key), "duration", time.Since(startTime))
	}()

	hostObj, err := ctrl.hostManifestLister.Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Manifest has been deleted", "manifest", klog.KRef("", name))
		obj, err := ctrl.manifestLister.Get(name)
		if errors.IsNotFound(err) {
			return nil
		}
		oldManifest := obj.(*unstructured.Unstructured)
		return ctrl.manifestClient.Delete(ctx, oldManifest.GetName(), metav1.DeleteOptions{})
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	hostManifest := hostObj.DeepCopyObject().(*unstructured.Unstructured)
	dropInvaildFields(hostManifest)

	existingObj, err := ctrl.manifestLister.Get(hostManifest.GetName())
	if errors.IsNotFound(err) {
		_, err := ctrl.manifestClient.Create(ctx, hostManifest, metav1.CreateOptions{})
		return err
	}
	existing := existingObj.(*unstructured.Unstructured)
	hostManifest.SetUID(existing.GetUID())
	hostManifest.SetResourceVersion(existing.GetResourceVersion())
	_, err = ctrl.manifestClient.Update(ctx, hostManifest, metav1.UpdateOptions{})
	return err
}
