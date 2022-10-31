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

package validate

import (
	"context"
	"encoding/json"
	"fmt"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/openapi"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/yaml"

	"github.com/firefly-io/firefly/pkg/fkactl/pkg/overridemanager"
	"github.com/firefly-io/firefly/pkg/util/diff"
)

// ValidateOptions defines flags and other configuration parameters for the `validate` command
type ValidateOptions struct {
	genericclioptions.IOStreams

	Validator     validation.Schema
	Builder       *resource.Builder
	Mapper        meta.RESTMapper
	DynamicClient dynamic.Interface
	OpenAPISchema openapi.Resources

	KarmadaClient karmadaversioned.Interface

	Namespace        string
	EnforceNamespace bool

	FilenameOptions resource.FilenameOptions
	Selector        string

	// Objects (and some denormalized data) which are to be
	// applied. The standard way to fill in this structure
	// is by calling "GetObjects()", which will use the
	// resource builder if "objectsCached" is false. The other
	// way to set this field is to use "SetObjects()".
	// Subsequent calls to "GetObjects()" after setting would
	// not call the resource builder; only return the set objects.
	objects       []*resource.Info
	objectsCached bool
}

var (
	validateLong = templates.LongDesc(`
		Validate a configuration to a resource by filename or stdin.

		JSON and YAML formats are accepted.`)

	validateExample = templates.Examples(`
		# Validate the configuration in deployment.json to a deployment.
		fkactl validate -f ./deployment.json

		# Validate resources from a directory containing kustomization.yaml - e.g. dir/kustomization.yaml.
		fkactl validate -k dir/

		# Validate the JSON passed into stdin to a deployment.
		cat deployment.json | fkactl validate -f -`)
)

// NewValidateOptions creates new ApplyOptions for the `validate` command
func NewValidateOptions(ioStreams genericclioptions.IOStreams) *ValidateOptions {
	return &ValidateOptions{
		IOStreams: ioStreams,

		objects:       []*resource.Info{},
		objectsCached: false,
	}
}

// NewCmdValidate creates the `validate` command
func NewCmdValidate(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewValidateOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "validate (-f FILENAME | -k DIRECTORY)",
		DisableFlagsInUseLine: true,
		Short:                 "Validate a configuration to a resource by filename or stdin",
		Long:                  validateLong,
		Example:               validateExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd))
			cmdutil.CheckErr(o.Run())
		},
	}

	cmdutil.AddFilenameOptionFlags(cmd, &o.FilenameOptions, "to use to validate the resource")
	return cmd
}

// Complete verifies if ValidateOptions are valid and without conflicts.
func (o *ValidateOptions) Complete(f cmdutil.Factory, cmd *cobra.Command) error {
	restConfig, err := f.ToRESTConfig()
	if err != nil {
		return err
	}

	o.KarmadaClient, err = karmadaversioned.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}

	o.OpenAPISchema, _ = f.OpenAPISchema()
	fieldValidationVerifier := resource.NewQueryParamVerifier(dynamicClient, f.OpenAPIGetter(), resource.QueryParamFieldValidation)
	o.Validator, err = f.Validator(metav1.FieldValidationStrict, fieldValidationVerifier)
	if err != nil {
		return err
	}
	o.Builder = f.NewBuilder()
	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		return err
	}

	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	return nil
}

// GetObjects returns a (possibly cached) version of all the valid objects to apply
// as a slice of pointer to resource.Info and an error if one or more occurred.
// IMPORTANT: This function can return both valid objects AND an error, since
// "ContinueOnError" is set on the builder. This function should not be called
// until AFTER the "complete" and "validate" methods have been called to ensure that
// the ValidateOptions is filled in and valid.
func (o *ValidateOptions) GetObjects() ([]*resource.Info, error) {
	var err error = nil
	if !o.objectsCached {
		// include the uninitialized objects by default if --prune is true
		// unless explicitly set --include-uninitialized=false
		r := o.Builder.
			Unstructured().
			ContinueOnError().
			NamespaceParam(o.Namespace).DefaultNamespace().
			FilenameParam(o.EnforceNamespace, &o.FilenameOptions).
			LabelSelectorParam(o.Selector).
			Flatten().
			Do()
		o.objects, err = r.Infos()
		o.objectsCached = true
	}
	return o.objects, err
}

// Run executes the `validate` command.
func (o *ValidateOptions) Run() error {
	// Generates the objects using the resource builder if they have not
	// already been stored by calling "SetObjects()" in the pre-processor.
	errs := []error{}
	infos, err := o.GetObjects()
	if err != nil {
		return err
	}

	if len(infos) == 0 && len(errs) == 0 {
		return fmt.Errorf("no objects passed to validate")
	}

	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	resources, cops, ops, err := o.splitResourcesAndOverrides(infos)
	if err != nil {
		return err
	}
	klog.Infoln("all original objects are valid")

	overrideManager := overridemanager.New(o.KarmadaClient, cops, ops)

	clusters, err := o.KarmadaClient.ClusterV1alpha1().Clusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, resource := range resources {
		for _, cluster := range clusters.Items {
			computed := resource.DeepCopy()
			cops, ops, err := overrideManager.ApplyOverridePolicies(computed, cluster.Name)
			if err != nil {
				klog.Errorf("failed to generate manifest for the %s resource (%s/%s): %v", resource.GetKind(), resource.GetNamespace(), resource.GetName(), err)
				return err
			}
			raw, _ := computed.MarshalJSON()
			if err := o.Validator.ValidateBytes(raw); err != nil {
				klog.Infof("generated resource for cluster %s \n%s\n", cluster.Name, diff.PrettyYAML(resource, computed))
				return err
			}
			if equality.Semantic.Equalities.DeepEqual(resource, computed) {
				continue
			}
			if cops != nil {
				data, _ := yaml.Marshal(cops)
				klog.V(4).Infof("Applied ClusterOverridePolicies:\n%s", data)
			}
			if ops != nil {
				data, _ := yaml.Marshal(ops)
				klog.V(4).Infof("AppliedOverridePolicies:\n%s", data)
			}
		}
	}

	klog.Infoln("all generated objects are valid")
	return nil
}

func (o *ValidateOptions) splitResourcesAndOverrides(infos []*resource.Info) ([]*unstructured.Unstructured, []policyv1alpha1.ClusterOverridePolicy, []policyv1alpha1.OverridePolicy, error) {
	resouceRefs := sets.NewString()
	resources := []*unstructured.Unstructured{}
	cops := []policyv1alpha1.ClusterOverridePolicy{}
	ops := []policyv1alpha1.OverridePolicy{}
	for _, info := range infos {
		data, err := json.Marshal(info.Object)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := o.Validator.ValidateBytes(data); err != nil {
			return nil, nil, nil, err
		}

		resource := &unstructured.Unstructured{}
		resource.UnmarshalJSON(data)

		gr := info.Mapping.Resource.GroupResource()
		fmt.Println(gr)
		switch gr.String() {
		case "overridepolicies.policy.karmada.io":
			op := policyv1alpha1.OverridePolicy{}
			if err := convertToTypedObject(resource, &op); err != nil {
				return nil, nil, nil, err
			}
			ops = append(ops, op)
		case "clusteroverridepolicies.policy.karmada.io":
			cop := policyv1alpha1.ClusterOverridePolicy{}
			if err := convertToTypedObject(resource, &cop); err != nil {
				return nil, nil, nil, err
			}
			cops = append(cops, cop)
		case "clusterpropagationpolicies.policy.karmada.io", "propagationpolicies.policy.karmada.io":
		default:
			ref := fmt.Sprintf("%s%s%s", info.Mapping.GroupVersionKind.String(), info.Name, info.Namespace)
			resouceRefs.Insert(ref)
			resources = append(resources, resource)
		}
	}

	// todo: check resources which is referenced by any override policy
	return resources, cops, ops, nil
}

// convertToTypedObject converts an unstructured object to typed.
func convertToTypedObject(in, out interface{}) error {
	switch v := in.(type) {
	case *unstructured.Unstructured:
		return runtime.DefaultUnstructuredConverter.FromUnstructured(v.UnstructuredContent(), out)
	case map[string]interface{}:
		return runtime.DefaultUnstructuredConverter.FromUnstructured(v, out)
	case *unstructured.UnstructuredList:
		return runtime.DefaultUnstructuredConverter.FromUnstructured(v.UnstructuredContent(), out)
	default:
		return fmt.Errorf("convert object must be pointer of unstructured or map[string]interface{}")
	}
}
