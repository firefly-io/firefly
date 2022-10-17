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

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	workv1alpha1 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	toolkitv1alpha1 "github.com/carlory/firefly/pkg/karmada/apis/toolkit/v1alpha1"
	"github.com/carlory/firefly/pkg/karmada/util"
)

func (ctrl *FooController) buildWorks(ctx context.Context, foo *toolkitv1alpha1.Foo, clusters []*clusterv1alpha1.Cluster) error {
	// convert manifest to unstructured.Unstructured
	workloads := make([]*unstructured.Unstructured, 0, len(foo.Spec.Manifests))
	for _, manifest := range foo.Spec.Manifests {
		workload := &unstructured.Unstructured{}
		err := workload.UnmarshalJSON(manifest.Raw)
		if err != nil {
			klog.ErrorS(err, "Failed to convert manifest to unstructured", "manifest", manifest.String())
			return err
		}
		workloads = append(workloads, workload)
	}

	// build works
	for _, workload := range workloads {
		if err := ctrl.applyAPIResources(ctx, foo, workload); err != nil {
			return err
		}
		for _, cluster := range clusters {
			if !cluster.DeletionTimestamp.IsZero() {
				continue
			}
			if err := ctrl.buildWork(ctx, foo, cluster, workload); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyAPIResources applies crds resource into the karmada controlplane.
func (ctrl *FooController) applyAPIResources(ctx context.Context, foo *toolkitv1alpha1.Foo, workload *unstructured.Unstructured) error {
	// Get the resource's GVR and scope
	gvr, ok := ctrl.tryRecognizeResource(ctx, workload)
	if !ok {
		klog.InfoS("Workload is unrecognized by the karmada controlplane", "workload", klog.KObj(workload), "gvk", workload.GroupVersionKind())
		return nil
	}

	if gvr.Group == "apiextensions.k8s.io" && gvr.Resource == "customresourcedefinitions" {
		// If the workload is a CRD, we should also create it on the karmada controlplane.
		// Otherwise, the karmada controlplane will not recognize the corresponding customresource.
		if err := ctrl.createOrUpdateCRD(ctx, foo, gvr, workload); err != nil {
			return err
		}
	}
	return nil
}

// createOrUpdateCRD creates or updates the given crd.
// If the workload is already exist and it's created by the foo, it will be updated.
// otherwise, we will report a conflict event about it on the foo and skip it.
func (ctrl *FooController) createOrUpdateCRD(ctx context.Context, foo *toolkitv1alpha1.Foo, gvr schema.GroupVersionResource, crd *unstructured.Unstructured) error {
	// Deep-copy otherwise we are mutating the original.
	clone := crd.DeepCopy()

	got, err := ctrl.dynamicClient.Resource(gvr).Get(ctx, crd.GetName(), metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// Set the labels to indicate that the workload is created by the foo.
		util.MergeLabel(clone, toolkitv1alpha1.FooNamespaceLabel, clone.GetNamespace())
		util.MergeLabel(clone, toolkitv1alpha1.FooNameLabel, clone.GetName())
		_, err := ctrl.dynamicClient.Resource(gvr).Create(ctx, clone, metav1.CreateOptions{})
		return err
	}

	// If the workload is already exist and it isn't created by the foo, we should skip it.
	labels := got.GetLabels()
	if labels == nil {
		return nil
	}
	namespace := labels[toolkitv1alpha1.FooNamespaceLabel]
	name := labels[toolkitv1alpha1.FooNameLabel]
	if namespace != foo.Namespace || name != foo.Name {
		return nil
	}

	clone.SetResourceVersion(got.GetResourceVersion())
	_, err = ctrl.dynamicClient.Resource(gvr).Update(ctx, clone, metav1.UpdateOptions{})
	return err
}

// tryRecognizeResource tries to recognize the resource from the given manifest. If the resource is unrecognized, it doesn't mean that the resource is invalid.
// If the resource is recognized, it will return the resource's GVR.
func (ctrl *FooController) tryRecognizeResource(ctx context.Context, workload *unstructured.Unstructured) (schema.GroupVersionResource, bool) {
	gvk := workload.GroupVersionKind()
	mapping, err := ctrl.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, false
	}
	return mapping.Resource, true
}

func (ctrl *FooController) buildWork(ctx context.Context, foo *toolkitv1alpha1.Foo, cluster *clusterv1alpha1.Cluster, workload *unstructured.Unstructured) error {
	workloadJSON, err := workload.MarshalJSON()
	if err != nil {
		klog.ErrorS(err, "Marshal workload to json failed", "workload", workload)
		return err
	}

	workNamespace, err := util.GenerateExecutionSpaceName(cluster.Name)
	if err != nil {
		klog.ErrorS(err, "Generate execution space name failed", "cluster", cluster.Name)
		return err
	}
	workName := util.GenerateWorkName(workload.GetKind(), workload.GetName(), workload.GetNamespace())

	work := &workv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workName,
			Namespace: workNamespace,
			Labels: map[string]string{
				toolkitv1alpha1.FooNamespaceLabel: foo.Namespace,
				toolkitv1alpha1.FooNameLabel:      foo.Name,
			},
			Finalizers: []string{util.ExecutionControllerFinalizer},
		},
		Spec: workv1alpha1.WorkSpec{
			Workload: workv1alpha1.WorkloadTemplate{
				Manifests: []workv1alpha1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Raw: workloadJSON,
						},
					},
				},
			},
		},
	}
	return ctrl.createOrUpdateWork(ctx, work)
}

func (ctrl *FooController) createOrUpdateWork(ctx context.Context, work *workv1alpha1.Work) error {
	existing, err := ctrl.worksLister.Works(work.Namespace).Get(work.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to get work", "work", klog.KObj(work))
			return err
		}
		_, err := ctrl.karmadaClient.WorkV1alpha1().Works(work.Namespace).Create(ctx, work, metav1.CreateOptions{})
		klog.ErrorS(err, "Failed to create work", "work", klog.KObj(work))
		return err
	}

	if equality.Semantic.DeepEqual(existing, work) {
		return nil
	}

	work.ResourceVersion = existing.ResourceVersion
	_, err = ctrl.karmadaClient.WorkV1alpha1().Works(work.Namespace).Update(ctx, work, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to update work", "work", klog.KObj(work))
		return err
	}
	return nil
}
