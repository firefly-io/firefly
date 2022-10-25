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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/carlory/firefly/pkg/scheme"
)

// NewClusterRefController returns a new *Controller.
func NewClusterRefController(
	client clientset.Interface,
	clustersInformer informers.GenericInformer,
	configMapInformer coreinformers.ConfigMapInformer,
	karmadaNamespace string,
	fireflyClient clientset.Interface) (*ClusterRefController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "kubean-clusterref-controller"})

	if client != nil && client.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("clusterref_controller", client.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &ClusterRefController{
		karmadaNamespace: karmadaNamespace,
		client:           client,
		clustersLister:   clustersInformer.Lister(),
		clustersSynced:   clustersInformer.Informer().HasSynced,
		configMapLister:  configMapInformer.Lister(),
		configMapSynced:  configMapInformer.Informer().HasSynced,
		fireflyClient:    fireflyClient,
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "configmap"),
		workerLoopPeriod: time.Second,
		eventBroadcaster: broadcaster,
		eventRecorder:    recorder,
	}

	clustersInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addCluster,
		UpdateFunc: ctrl.updateCluster,
	}, 15*time.Second)

	return ctrl, nil
}

type ClusterRefController struct {
	client clientset.Interface

	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	clustersLister cache.GenericLister
	clustersSynced cache.InformerSynced

	configMapLister corelisters.ConfigMapLister
	configMapSynced cache.InformerSynced

	karmadaNamespace string
	fireflyClient    clientset.Interface

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
func (ctrl *ClusterRefController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.client.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting kubean clusterref controller")
	defer klog.Infof("Shutting down kubean clusterref controller")

	if !cache.WaitForNamedCacheSync("configmap", ctx.Done(), ctrl.clustersSynced, ctrl.configMapSynced) {
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
func (ctrl *ClusterRefController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *ClusterRefController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.sync(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *ClusterRefController) addCluster(obj interface{}) {
	cluster := obj.(*unstructured.Unstructured)

	namespace, name, ok := getHostsConfRef(cluster)
	if ok {
		cm, err := ctrl.configMapLister.ConfigMaps(namespace).Get(name)
		if err != nil {
			klog.ErrorS(err, "unable to get the host config for a cluster", "cluster", cluster.GetName())
		}
		if cm != nil {
			klog.V(4).InfoS("Adding configmap", "cluster", klog.KObj(cluster), "namespace", namespace, "name", name)
			ctrl.enqueue(cm)
		}
	}

	namespace, name, ok = getVarsConfRef(cluster)
	if ok {
		cm, err := ctrl.configMapLister.ConfigMaps(namespace).Get(name)
		if err != nil {
			klog.ErrorS(err, "unable to get the vars config for a cluster", "cluster", cluster.GetName())
		}
		if cm != nil {
			klog.V(4).InfoS("Adding configmap", "cluster", klog.KObj(cluster), "namespace", namespace, "name", name)
			ctrl.enqueue(cm)
		}
	}
}

func (ctrl *ClusterRefController) updateCluster(old, cur interface{}) {
	ctrl.addCluster(cur)
}

func (ctrl *ClusterRefController) enqueue(cm *corev1.ConfigMap) {
	key, err := cache.MetaNamespaceKeyFunc(cm)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctrl.queue.Add(key)
}

func (ctrl *ClusterRefController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	ns, name, keyErr := cache.SplitMetaNamespaceKey(key.(string))
	if keyErr != nil {
		klog.ErrorS(err, "Failed to split meta cache key", "cacheKey", key)
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing configmap, retrying", "configmap", klog.KRef(ns, name), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping configmap out of the queue", "configmap", klog.KRef(ns, name), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *ClusterRefController) sync(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.ErrorS(err, "Failed to split meta cache key", "cacheKey", key)
		return err
	}

	startTime := time.Now()
	klog.V(4).InfoS("Started syncing configmap", "namespace", namespace, "configmap", name, "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing configmap", "namespace", namespace, "configmap", name, "duration", time.Since(startTime))
	}()

	cm, err := ctrl.configMapLister.ConfigMaps(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.V(2).InfoS("Cluster has been deleted", "cluster", name)
		return nil
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	cm = cm.DeepCopy()

	// examine DeletionTimestamp to determine if object is under deletion
	if cm.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(cm, ClusterControllerFinalizerName) {
			controllerutil.AddFinalizer(cm, ClusterControllerFinalizerName)
			cm, err = ctrl.client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(cm, ClusterControllerFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := ctrl.deleteUnableGCResources(ctx, cm); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(cm, ClusterControllerFinalizerName)
			_, err := ctrl.client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			// Stop reconciliation as the item is being deleted
			return nil
		}
	}

	klog.InfoS("Syncing configmap", "configmap", klog.KObj(cm))

	hostConfigMap := convert_confimap_from_karmada_to_host(ctrl.karmadaNamespace, cm)
	klog.InfoS("convert the configmap from karmada to host", "karmada", klog.KObj(cm), "host", klog.KObj(hostConfigMap))
	dropInvaildFields(hostConfigMap)

	existing, err := ctrl.fireflyClient.CoreV1().ConfigMaps(hostConfigMap.Namespace).Get(ctx, hostConfigMap.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get configmap from host", "configmap", klog.KObj(hostConfigMap))
			return err
		}
		klog.InfoS("Creating configmap on the host cluster", klog.KObj(hostConfigMap))
		_, err := ctrl.fireflyClient.CoreV1().ConfigMaps(hostConfigMap.Namespace).Create(ctx, hostConfigMap, metav1.CreateOptions{})
		if err != nil {
			klog.ErrorS(err, "failed to create configmap on the host", "configmap", klog.KObj(hostConfigMap))
			return err
		}
		return nil
	}

	hostConfigMap.SetUID(existing.GetUID())
	hostConfigMap.SetResourceVersion(existing.GetResourceVersion())
	_, err = ctrl.fireflyClient.CoreV1().ConfigMaps(hostConfigMap.Namespace).Update(ctx, hostConfigMap, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to update configmap on the host", "configmap", klog.KObj(hostConfigMap))
	}
	return err
}

func (ctrl *ClusterRefController) deleteUnableGCResources(ctx context.Context, cm *corev1.ConfigMap) error {
	hostCM := convert_confimap_from_karmada_to_host(ctrl.karmadaNamespace, cm)
	err := ctrl.fireflyClient.CoreV1().ConfigMaps(hostCM.Namespace).Delete(ctx, hostCM.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}
