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

package foo

import (
	"context"
	"fmt"
	"time"

	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	clusterinformers "github.com/karmada-io/karmada/pkg/generated/informers/externalversions/cluster/v1alpha1"
	workinformers "github.com/karmada-io/karmada/pkg/generated/informers/externalversions/work/v1alpha1"
	clusterlisters "github.com/karmada-io/karmada/pkg/generated/listers/cluster/v1alpha1"
	worklisters "github.com/karmada-io/karmada/pkg/generated/listers/work/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	toolkitv1alpha1 "github.com/carlory/firefly/pkg/karmada/apis/toolkit/v1alpha1"
	fireflyclient "github.com/carlory/firefly/pkg/karmada/generated/clientset/versioned"
	toolkitinformers "github.com/carlory/firefly/pkg/karmada/generated/informers/externalversions/toolkit/v1alpha1"
	toolkitlisters "github.com/carlory/firefly/pkg/karmada/generated/listers/toolkit/v1alpha1"
	"github.com/carlory/firefly/pkg/scheme"
)

const (
	// maxRetries is the number of times a foo will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a foo.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15

	// name of the foo controller finalizer
	FooControllerFinalizerName = "foo.tooklit.firefly.io/finalizer"
)

// NewFooController returns a new *Controller.
func NewFooController(
	restMapper meta.RESTMapper,
	dynamicClient dynamic.Interface,
	client clientset.Interface,
	karmadaClient karmadaversioned.Interface,
	karmadaFireflyClient fireflyclient.Interface,
	fooInformer toolkitinformers.FooInformer,
	clusterInformer clusterinformers.ClusterInformer,
	workInformer workinformers.WorkInformer) (*FooController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "foo-controller"})

	if client != nil && client.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("foo_controller", client.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &FooController{
		restMapper:           restMapper,
		dynamicClient:        dynamicClient,
		client:               client,
		karmadaClient:        karmadaClient,
		karmadaFireflyClient: karmadaFireflyClient,
		foosLister:           fooInformer.Lister(),
		foosSynced:           fooInformer.Informer().HasSynced,
		clustersLister:       clusterInformer.Lister(),
		clustersSynced:       clusterInformer.Informer().HasSynced,
		worksLister:          workInformer.Lister(),
		worksSynced:          workInformer.Informer().HasSynced,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "foo"),
		workerLoopPeriod:     time.Second,
		eventBroadcaster:     broadcaster,
		eventRecorder:        recorder,
	}

	fooInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addFoo,
		UpdateFunc: ctrl.updateFoo,
		DeleteFunc: ctrl.deleteFoo,
	})

	return ctrl, nil
}

type FooController struct {
	restMapper           meta.RESTMapper
	dynamicClient        dynamic.Interface
	client               clientset.Interface
	karmadaClient        karmadaversioned.Interface
	karmadaFireflyClient fireflyclient.Interface
	eventBroadcaster     record.EventBroadcaster
	eventRecorder        record.EventRecorder

	foosLister     toolkitlisters.FooLister
	foosSynced     cache.InformerSynced
	clustersLister clusterlisters.ClusterLister
	clustersSynced cache.InformerSynced
	worksLister    worklisters.WorkLister
	worksSynced    cache.InformerSynced

	// Foo that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// foo will be handled in parallel.
func (ctrl *FooController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.client.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting foo controller")
	defer klog.Infof("Shutting down foo controller")

	if !cache.WaitForNamedCacheSync("foo", ctx.Done(), ctrl.foosSynced, ctrl.clustersSynced, ctrl.worksSynced) {
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
func (ctrl *FooController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *FooController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncFoo(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *FooController) addFoo(obj interface{}) {
	foo := obj.(*toolkitv1alpha1.Foo)
	klog.V(4).InfoS("Adding foo", "foo", klog.KObj(foo))
	ctrl.enqueue(foo)
}

func (ctrl *FooController) updateFoo(old, cur interface{}) {
	oldFoo := old.(*toolkitv1alpha1.Foo)
	curFoo := cur.(*toolkitv1alpha1.Foo)
	klog.V(4).InfoS("Updating foo", "foo", klog.KObj(oldFoo))
	ctrl.enqueue(curFoo)
}

func (ctrl *FooController) deleteFoo(obj interface{}) {
	foo, ok := obj.(*toolkitv1alpha1.Foo)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		foo, ok = tombstone.Obj.(*toolkitv1alpha1.Foo)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Foo %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting foo", "foo", klog.KObj(foo))
	ctrl.enqueue(foo)
}

func (ctrl *FooController) enqueue(foo *toolkitv1alpha1.Foo) {
	key, err := cache.MetaNamespaceKeyFunc(foo)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *FooController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	ns, name, keyErr := cache.SplitMetaNamespaceKey(key.(string))
	if keyErr != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing foo, retrying", "foo", klog.KRef(ns, name), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping foo out of the queue", "foo", klog.KRef(ns, name), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *FooController) syncFoo(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing foo", "foo", klog.KRef(namespace, name), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing foo", "foo", klog.KRef(namespace, name), "duration", time.Since(startTime))
	}()

	foo, err := ctrl.foosLister.Foos(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Foo has been deleted", "foo", klog.KRef(namespace, name))
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	foo = foo.DeepCopy()

	// examine DeletionTimestamp to determine if object is under deletion
	if foo.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(foo, FooControllerFinalizerName) {
			controllerutil.AddFinalizer(foo, FooControllerFinalizerName)
			foo, err = ctrl.karmadaFireflyClient.ToolkitV1alpha1().Foos(foo.Namespace).Update(ctx, foo, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(foo, FooControllerFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := ctrl.deleteUnableGCResources(ctx, foo); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(foo, FooControllerFinalizerName)
			_, err := ctrl.karmadaFireflyClient.ToolkitV1alpha1().Foos(foo.Namespace).Update(ctx, foo, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			// Stop reconciliation as the item is being deleted
			return nil
		}
	}

	klog.InfoS("Syncing foo", "foo", klog.KObj(foo))

	clusters, err := ctrl.clustersLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "Failed to list clusters")
		return err
	}

	return ctrl.buildWorks(ctx, foo, clusters)
}

func (ctrl *FooController) deleteUnableGCResources(ctx context.Context, foo *toolkitv1alpha1.Foo) error {
	selector := labels.SelectorFromSet(labels.Set{
		toolkitv1alpha1.FooNamespaceLabel: foo.Namespace,
		toolkitv1alpha1.FooNameLabel:      foo.Name,
	})

	// clear crds
	gvr := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	crds, err := ctrl.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		klog.ErrorS(err, "Failed to list crds", "foo", klog.KObj(foo), "selector", selector)
		return err
	}
	for _, crd := range crds.Items {
		// todo remove cr
		err := ctrl.dynamicClient.Resource(gvr).Delete(ctx, crd.GetName(), metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// clear works
	works, err := ctrl.worksLister.List(selector)
	if err != nil {
		klog.ErrorS(err, "Failed to list works from cache", "foo", klog.KObj(foo), "selector", selector)
		return err
	}

	klog.V(4).InfoS("Deleting works", "foo", klog.KObj(foo), "workCount", len(works))

	for _, work := range works {
		err := ctrl.karmadaClient.WorkV1alpha1().Works(work.Namespace).Delete(ctx, work.Name, metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete work", "work", klog.KObj(work))
				return err
			}
		}
	}
	return nil
}
