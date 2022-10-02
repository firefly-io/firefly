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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	fireflyclient "github.com/carlory/firefly/pkg/generated/clientset/versioned"
	installinformers "github.com/carlory/firefly/pkg/generated/informers/externalversions/install/v1alpha1"
	installlisters "github.com/carlory/firefly/pkg/generated/listers/install/v1alpha1"
	"github.com/carlory/firefly/pkg/util"
)

const (
	// maxRetries is the number of times a karmada will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a karmada.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15

	// name of the karmada controller finalizer
	KarmadaControllerFinalizerName = "karmada.install.firefly.io/finalizer"
)

// NewKarmadaController returns a new *Controller.
func NewKarmadaController(
	client clientset.Interface,
	fireflyClient fireflyclient.Interface,
	karmadaInformer installinformers.KarmadaInformer) (*KarmadaController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "karmada-controller"})

	if client != nil && client.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("karmada_controller", client.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &KarmadaController{
		client:           client,
		fireflyClient:    fireflyClient,
		karmadasLister:   karmadaInformer.Lister(),
		karmadasSynced:   karmadaInformer.Informer().HasSynced,
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "karmada"),
		workerLoopPeriod: time.Second,
		eventBroadcaster: broadcaster,
		eventRecorder:    recorder,
	}

	karmadaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addKarmada,
		UpdateFunc: ctrl.updateKarmada,
		DeleteFunc: ctrl.deleteKarmada,
	})

	return ctrl, nil
}

type KarmadaController struct {
	client           clientset.Interface
	fireflyClient    fireflyclient.Interface
	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	karmadasLister installlisters.KarmadaLister
	karmadasSynced cache.InformerSynced

	// Karmada that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// karmada will be handled in parallel.
func (ctrl *KarmadaController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.client.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting karmada controller")
	defer klog.Infof("Shutting down karmada controller")

	if !cache.WaitForNamedCacheSync("karmada", ctx.Done(), ctrl.karmadasSynced) {
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
func (ctrl *KarmadaController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *KarmadaController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncKarmada(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *KarmadaController) addKarmada(obj interface{}) {
	karmada := obj.(*installv1alpha1.Karmada)
	klog.V(4).InfoS("Adding karmada", "karmada", klog.KObj(karmada))
	ctrl.enqueue(karmada)
}

func (ctrl *KarmadaController) updateKarmada(old, cur interface{}) {
	oldKarmada := old.(*installv1alpha1.Karmada)
	curKarmada := cur.(*installv1alpha1.Karmada)
	klog.V(4).InfoS("Updating karmada", "karmada", klog.KObj(oldKarmada))
	ctrl.enqueue(curKarmada)
}

func (ctrl *KarmadaController) deleteKarmada(obj interface{}) {
	karmada, ok := obj.(*installv1alpha1.Karmada)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		karmada, ok = tombstone.Obj.(*installv1alpha1.Karmada)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Karmada %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting karmada", "karmada", klog.KObj(karmada))
	ctrl.enqueue(karmada)
}

func (ctrl *KarmadaController) enqueue(karmada *installv1alpha1.Karmada) {
	key, err := cache.MetaNamespaceKeyFunc(karmada)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *KarmadaController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	ns, name, keyErr := cache.SplitMetaNamespaceKey(key.(string))
	if keyErr != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing karmada, retrying", "karmada", klog.KRef(ns, name), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping karmada out of the queue", "karmada", klog.KRef(ns, name), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *KarmadaController) syncKarmada(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta namespace cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing karmada", "karmada", klog.KRef(namespace, name), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing karmada", "karmada", klog.KRef(namespace, name), "duration", time.Since(startTime))
	}()

	karmada, err := ctrl.karmadasLister.Karmadas(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Karmada has been deleted", "karmada", klog.KRef(namespace, name))
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	karmada = karmada.DeepCopy()

	// examine DeletionTimestamp to determine if object is under deletion
	if karmada.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(karmada, KarmadaControllerFinalizerName) {
			controllerutil.AddFinalizer(karmada, KarmadaControllerFinalizerName)
			karmada, err = ctrl.fireflyClient.InstallV1alpha1().Karmadas(karmada.Namespace).Update(ctx, karmada, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(karmada, KarmadaControllerFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := ctrl.deleteUnableGCResources(karmada); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(karmada, KarmadaControllerFinalizerName)
			_, err := ctrl.fireflyClient.InstallV1alpha1().Karmadas(karmada.Namespace).Update(ctx, karmada, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			// Stop reconciliation as the item is being deleted
			return nil
		}
	}

	klog.InfoS("Syncing karmada", "karmada", klog.KObj(karmada))

	if err := ctrl.genCerts(karmada, nil); err != nil {
		klog.ErrorS(err, "Failed to generate certs", "namespace", namespace)
		return err
	}

	if err := ctrl.EnsureEtcd(karmada); err != nil {
		return err
	}

	if err := ctrl.EnsureAPIServer(karmada); err != nil {
		return err
	}

	if err := ctrl.EnsureControllerManager(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureScheduler(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureAPIServer(karmada *installv1alpha1.Karmada) error {
	kubeClient, err := ctrl.EnsureKubeAPIServer(karmada)
	if err != nil {
		return err
	}

	klog.InfoS("karmada-apiserver is ready", "karmada", klog.KObj(karmada))

	_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "karmada-system"}}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	if err := ctrl.EnsureKarmadaAggregatedAPIServer(karmada); err != nil {
		return err
	}

	if err := ctrl.EnsureKarmadaCRDs(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureKaramdaWebhook(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureControllerManager(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureKubeControllerManager(karmada); err != nil {
		return err
	}

	if err := ctrl.EnsureKarmadaControllerManager(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureFireflyKarmadaManager(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) EnsureScheduler(karmada *installv1alpha1.Karmada) error {
	if err := ctrl.EnsureKarmadaScheduler(karmada); err != nil {
		return err
	}
	if err := ctrl.EnsureKarmadaDescheduler(karmada); err != nil {
		return err
	}
	return nil
}

func (ctrl *KarmadaController) deleteUnableGCResources(karmada *installv1alpha1.Karmada) error {
	bindingName := util.ComponentName(constants.FireflyComponentKarmadaManager, karmada.Name)
	err := ctrl.client.RbacV1beta1().ClusterRoleBindings().Delete(context.Background(), bindingName, metav1.DeleteOptions{})
	return client.IgnoreNotFound(err)
}
