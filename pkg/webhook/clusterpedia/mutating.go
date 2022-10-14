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
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

// MutatingAdmission mutates API request if necessary.
type MutatingAdmission struct {
	decoder *admission.Decoder
}

// Check if our MutatingAdmission implements necessary interface
var _ admission.Handler = &MutatingAdmission{}
var _ admission.DecoderInjector = &MutatingAdmission{}

// NewMutatingHandler builds a new admission.Handler.
func NewMutatingHandler() admission.Handler {
	return &MutatingAdmission{}
}

// Handle yields a response to an AdmissionRequest.
func (a *MutatingAdmission) Handle(ctx context.Context, req admission.Request) admission.Response {
	clusterpedia := &installv1alpha1.Clusterpedia{}

	err := a.decoder.Decode(req, clusterpedia)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	klog.InfoS("Setting defaults", "clusterpedia", klog.KObj(clusterpedia))
	installv1alpha1.SetDefaults_Clusterpedia(clusterpedia)
	marshaledBytes, err := json.Marshal(clusterpedia)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledBytes)
}

// InjectDecoder implements admission.DecoderInjector interface.
// A decoder will be automatically injected.
func (a *MutatingAdmission) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
