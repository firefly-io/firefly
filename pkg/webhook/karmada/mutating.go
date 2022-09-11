package karmada

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
	karmada := &installv1alpha1.Karmada{}

	err := a.decoder.Decode(req, karmada)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	klog.InfoS("Setting defaults", "karmada", klog.KObj(karmada))
	installv1alpha1.SetDefaults_Karmada(karmada)
	marshaledBytes, err := json.Marshal(karmada)
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
