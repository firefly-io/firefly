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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	clusterNamePrefix         = "firefly-"
	hostsConfRefAnnotationKey = "kubean.firefly.io/hosts-conf-ref"
	varsConfRefAnnotationKey  = "kubean.firefly.io/vars-conf-ref"
)

// convert_kubean_cluster_karmada_to_host converts a kubean cluster on the karmada to a kubean cluster on the host.
// the namespace represents the namespace of the karmada components on the host. we don't want to use other namespace to
// create the kubean dependencies configmap on the host cluster.
func convert_kubean_cluster_karmada_to_host(karmadaNamespace string, in *unstructured.Unstructured) (out *unstructured.Unstructured) {
	out = in.DeepCopy()

	// add the firefly prefix to the name of the cluster to avoid the conflict with the cluster on the host.
	name := out.GetName()
	out.SetName(clusterNamePrefix + name)

	annotations := out.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// add the hostsConfRef annotation to the cluster. we use it to store the original hostsConfRef.
	hostsConfRef, _, _ := unstructured.NestedStringMap(out.Object, "spec", "hostsConfRef")
	if hostsConfRef != nil {
		namespace := hostsConfRef["namespace"]
		name := hostsConfRef["name"]
		annotations[hostsConfRefAnnotationKey] = namespace + "/" + name
		newHostsConfRef := map[string]interface{}{
			"namespace": karmadaNamespace,
			"name":      namespace + "-" + name,
		}
		unstructured.SetNestedField(out.Object, newHostsConfRef, "spec", "hostsConfRef")
	}

	// add the hostsConfRef annotation to the cluster. we use it to store the original varsConfRef.
	varsConfRef, _, _ := unstructured.NestedStringMap(out.Object, "spec", "varsConfRef")
	if varsConfRef != nil {
		namespace := varsConfRef["namespace"]
		name := varsConfRef["name"]
		annotations[varsConfRefAnnotationKey] = namespace + "/" + name
		newVarsConfRef := map[string]interface{}{
			"namespace": karmadaNamespace,
			"name":      namespace + "-" + name,
		}
		unstructured.SetNestedField(out.Object, newVarsConfRef, "spec", "varsConfRef")
	}

	out.SetAnnotations(annotations)

	return
}

// convert_kubean_cluster_karmada_to_host converts a kubean cluster on the karmada to a kubean cluster on the host.
func convert_kubean_cluster_host_to_karmada(in *unstructured.Unstructured) (out *unstructured.Unstructured) {
	out = in.DeepCopy()

	// get the original name of the cluster on a karmada.
	out.SetName(strings.TrimPrefix(out.GetName(), clusterNamePrefix))

	// get the original hostsConfRef and varsConfRef.
	annotations := out.GetAnnotations()
	if annotations == nil {
		return
	}

	hostsConfRefVals := strings.SplitN(annotations[hostsConfRefAnnotationKey], "/", 2)
	if len(hostsConfRefVals) == 2 {
		hostsConfRef := map[string]interface{}{
			"namespace": hostsConfRefVals[0],
			"name":      hostsConfRefVals[1],
		}
		unstructured.SetNestedField(out.Object, hostsConfRef, "spec", "hostsConfRef")
	}

	varsConfRefVals := strings.SplitN(annotations[varsConfRefAnnotationKey], "/", 2)
	if len(varsConfRefVals) == 2 {
		varsConfRef := map[string]interface{}{
			"namespace": varsConfRefVals[0],
			"name":      varsConfRefVals[1],
		}
		unstructured.SetNestedField(out.Object, varsConfRef, "spec", "varsConfRef")
	}
	return
}

// convert_confimap_from_karmada_to_host converts a confimap on the karmada to the host.
func convert_confimap_from_karmada_to_host(karmadaNamespace string, in *corev1.ConfigMap) (out *corev1.ConfigMap) {
	out = in.DeepCopy()
	out.Name = out.Namespace + "-" + out.Name
	out.Namespace = karmadaNamespace
	return
}

func getHostsConfRef(cluster *unstructured.Unstructured) (string, string, bool) {
	hostsConfRef, _, _ := unstructured.NestedStringMap(cluster.Object, "spec", "hostsConfRef")
	if hostsConfRef != nil {
		return hostsConfRef["namespace"], hostsConfRef["name"], true
	}
	return "", "", false
}

func getVarsConfRef(cluster *unstructured.Unstructured) (string, string, bool) {
	hostsConfRef, _, _ := unstructured.NestedStringMap(cluster.Object, "spec", "varsConfRef")
	if hostsConfRef != nil {
		return hostsConfRef["namespace"], hostsConfRef["name"], true
	}
	return "", "", false
}

func dropInvaildFields(obj metav1.Object) {
	obj.SetFinalizers(nil)
	obj.SetOwnerReferences(nil)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetOwnerReferences(nil)
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetDeletionTimestamp(nil)
	obj.SetGeneration(0)
	obj.SetGenerateName("")
}
