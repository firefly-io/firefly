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

package node

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"
)

const (
	// maxRetries is the number of times a node will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the
	// sequence of delays between successive queuings of a node.
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

// NewNodeController returns a new *Controller.
func NewNodeController(
	karmadaKubeClient clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	karmadaNodeInformer coreinformers.NodeInformer,
) (*NodeController, error) {
	broadcaster := record.NewBroadcaster()
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "node-controller"})

	if karmadaKubeClient != nil && karmadaKubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("node_controller", karmadaKubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	ctrl := &NodeController{
		karmadaKubeClient:  karmadaKubeClient,
		nodeLister:         nodeInformer.Lister(),
		nodeSynced:         nodeInformer.Informer().HasSynced,
		karmadaNodeLister:  karmadaNodeInformer.Lister(),
		karmadanNodeSynced: karmadaNodeInformer.Informer().HasSynced,
		queue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "node"),
		workerLoopPeriod:   time.Second,
		eventBroadcaster:   broadcaster,
		eventRecorder:      recorder,
	}
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addNode,
		UpdateFunc: ctrl.updateNode,
		DeleteFunc: ctrl.deleteNode,
	})
	karmadaNodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ctrl.updateNode,
		DeleteFunc: ctrl.deleteNode,
	})

	return ctrl, nil
}

type NodeController struct {
	karmadaKubeClient clientset.Interface
	eventBroadcaster  record.EventBroadcaster
	eventRecorder     record.EventRecorder

	nodeLister         corelisters.NodeLister
	nodeSynced         cache.InformerSynced
	karmadaNodeLister  corelisters.NodeLister
	karmadanNodeSynced cache.InformerSynced

	// Node that need to be updated. A channel is inappropriate here,
	// because it allows services with lots of pods to be serviced much
	// more often than services with few pods; it also would cause a
	// service that's inserted multiple times to be processed more than
	// necessary.
	queue workqueue.RateLimitingInterface

	// workerLoopPeriod is the time between worker runs. The workers process the queue of service and pod changes.
	workerLoopPeriod time.Duration
}

// Run will not return until stopCh is closed. workers determines how many
// node will be handled in parallel.
func (ctrl *NodeController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	// Start events processing pipelinctrl.
	ctrl.eventBroadcaster.StartStructuredLogging(0)
	ctrl.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: ctrl.karmadaKubeClient.CoreV1().Events("")})
	defer ctrl.eventBroadcaster.Shutdown()

	defer ctrl.queue.ShutDown()

	klog.Infof("Starting node controller")
	defer klog.Infof("Shutting down node controller")

	if !cache.WaitForNamedCacheSync("node", ctx.Done(), ctrl.nodeSynced, ctrl.karmadanNodeSynced) {
		return
	}

	nodes, _ := ctrl.karmadaNodeLister.List(labels.Everything())
	for _, node := range nodes {
		ctrl.addNode(node)
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
func (ctrl *NodeController) worker(ctx context.Context) {
	for ctrl.processNextWorkItem(ctx) {
	}
}

func (ctrl *NodeController) processNextWorkItem(ctx context.Context) bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.syncNode(ctx, key.(string))
	ctrl.handleErr(err, key)

	return true
}

func (ctrl *NodeController) addNode(obj interface{}) {
	node := obj.(*corev1.Node)
	klog.V(4).InfoS("Adding node", "node", klog.KObj(node))
	ctrl.enqueue(node)
}

func (ctrl *NodeController) updateNode(old, cur interface{}) {
	oldNode := old.(*corev1.Node)
	curNode := cur.(*corev1.Node)
	klog.V(4).InfoS("Updating node", "node", klog.KObj(oldNode))
	ctrl.enqueue(curNode)
}

func (ctrl *NodeController) deleteNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Node %#v", obj))
			return
		}
	}
	klog.V(4).InfoS("Deleting node", "node", klog.KObj(node))
	ctrl.enqueue(node)
}

func (ctrl *NodeController) enqueue(node *corev1.Node) {
	ctrl.queue.Add(node.Name)
}

func (ctrl *NodeController) handleErr(err error, key interface{}) {
	if err == nil || errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		ctrl.queue.Forget(key)
		return
	}

	if ctrl.queue.NumRequeues(key) < maxRetries {
		klog.V(2).InfoS("Error syncing node, retrying", "node", klog.KRef("", key.(string)), "err", err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	utilruntime.HandleError(err)
	klog.V(2).InfoS("Dropping node out of the queue", "node", klog.KRef("", key.(string)), "err", err)
	ctrl.queue.Forget(key)
}

func (ctrl *NodeController) syncNode(ctx context.Context, key string) error {
	startTime := time.Now()
	klog.V(4).InfoS("Started syncing node", "node", klog.KRef("", key), "startTime", startTime)
	defer func() {
		klog.V(4).InfoS("Finished syncing node", "node", klog.KRef("", key), "duration", time.Since(startTime))
	}()

	node, err := ctrl.nodeLister.Get(key)
	if errors.IsNotFound(err) || node.DeletionTimestamp != nil {
		klog.V(2).InfoS("Node has been deleted", "node", klog.KRef("", key))
		oldNode, err := ctrl.karmadaNodeLister.Get(key)
		if errors.IsNotFound(err) {
			return nil
		}
		return ctrl.karmadaKubeClient.CoreV1().Nodes().Delete(ctx, oldNode.Name, metav1.DeleteOptions{})
	}
	if err != nil {
		return err
	}

	// Deep-copy otherwise we are mutating our cache.
	// TODO: Deep-copy only when needed.
	clone := node.DeepCopy()
	dropInvaildFields(clone)

	karmadaNode, err := ctrl.karmadaNodeLister.Get(clone.Name)
	if errors.IsNotFound(err) {
		setKarmadaFields(nil, clone)
		_, err := ctrl.karmadaKubeClient.CoreV1().Nodes().Create(ctx, clone, metav1.CreateOptions{})
		return err
	}

	setKarmadaFields(karmadaNode, clone)
	_, err = ctrl.karmadaKubeClient.CoreV1().Nodes().Update(ctx, clone, metav1.UpdateOptions{})
	return err
}

func dropInvaildFields(node *corev1.Node) {
	node.Finalizers = []string{}
	node.OwnerReferences = nil
	node.ResourceVersion = ""
	node.UID = ""
	node.CreationTimestamp = metav1.Time{}
	node.DeletionTimestamp = nil
	node.Generation = 0
	node.GenerateName = ""
}

func setKarmadaFields(oldNode, newNode *corev1.Node) {
	if newNode.Labels == nil {
		newNode.Labels = make(map[string]string)
	}
	newNode.Labels["node-role.kubernetes.io/karmada-host"] = ""

	if oldNode == nil {
		return
	}
	newNode.ResourceVersion = oldNode.ResourceVersion
	newNode.UID = oldNode.UID
}
