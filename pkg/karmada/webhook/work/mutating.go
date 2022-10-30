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

package work

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	workv1alpha1 "github.com/karmada-io/karmada/pkg/apis/work/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util"
	"github.com/karmada-io/karmada/pkg/util/names"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// MutatingAdmission mutates API request if necessary.
type MutatingAdmission struct {
	client.Client
	decoder *admission.Decoder
}

// Check if our MutatingAdmission implements necessary interface
var _ admission.Handler = &MutatingAdmission{}
var _ admission.DecoderInjector = &MutatingAdmission{}

// Handle yields a response to an AdmissionRequest.
func (a *MutatingAdmission) Handle(ctx context.Context, req admission.Request) admission.Response {
	work := &workv1alpha1.Work{}

	err := a.decoder.Decode(req, work)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	clusterName, err := names.GetClusterName(work.Namespace)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	cluster, err := util.GetCluster(a, clusterName)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	apiEnablements := cluster.Status.APIEnablements

	var manifests []workv1alpha1.Manifest
	for _, manifest := range work.Spec.Workload.Manifests {
		workload := &unstructured.Unstructured{}
		err := workload.UnmarshalJSON(manifest.Raw)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		var (
			versions []string
			found    bool
		)

		curGVK := workload.GroupVersionKind()
		for _, apiEnablement := range apiEnablements {
			var (
				group   string
				version string
			)
			parts := strings.SplitN(apiEnablement.GroupVersion, "/", 2)
			if len(parts) == 2 {
				group = parts[0]
				version = parts[1]
			} else {
				version = parts[0]
			}
			if group != curGVK.Group {
				continue
			}
			for _, resource := range apiEnablement.Resources {
				if resource.Kind == curGVK.Kind {
					if version == curGVK.Version {
						found = true
						manifests = append(manifests, manifest)
					}
					versions = append(versions, version)
				}
			}
		}

		if found || len(versions) == 0 {
			continue
		}

		convertableGVs := legacyscheme.Scheme.VersionsForGroupKind(curGVK.GroupKind())
		convertableVersions := sets.NewString()
		for _, convertableGV := range convertableGVs {
			convertableVersions.Insert(convertableGV.Version)
		}

		var targetVersion string
		for _, version := range versions {
			if convertableVersions.Has(version) {
				targetVersion = version
				break
			}
		}

		if targetVersion == "" {
			manifests = append(manifests, manifest)
			continue
		}

		klog.InfoS("the target version of the manfiest is not supported by the cluster, will be converted to the prefered version", "currentVersion", curGVK.Version, "targetVersion", targetVersion)
		unversioned, err := legacyscheme.Scheme.UnsafeConvertToVersion(workload, schema.GroupVersion{Group: curGVK.Group, Version: runtime.APIVersionInternal})
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		versioned, err := legacyscheme.Scheme.UnsafeConvertToVersion(unversioned, schema.GroupVersion{Group: curGVK.Group, Version: targetVersion})
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		workloadJSON, err := json.Marshal(versioned)
		if err != nil {
			klog.ErrorS(err, "Failed to marshal workload of work(%s)", work.Name)
			return admission.Errored(http.StatusInternalServerError, err)
		}
		manifests = append(manifests, workv1alpha1.Manifest{RawExtension: runtime.RawExtension{Raw: workloadJSON}})
	}

	work.Spec.Workload.Manifests = manifests
	workData, err := json.Marshal(work)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, workData)
}

// InjectDecoder implements admission.DecoderInjector interface.
// A decoder will be automatically injected.
func (a *MutatingAdmission) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
