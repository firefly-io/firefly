package overridemanager

import (
	"context"
	"encoding/json"
	"sort"

	jsonpatch "github.com/evanphx/json-patch/v5"
	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util"
)

// OverrideManager managers override policies operation
type OverrideManager interface {
	// ApplyOverridePolicies overrides the object if one or more override policies exist and matches the target cluster.
	// For cluster scoped resource:
	// - Apply ClusterOverridePolicy by policies name in ascending
	// For namespaced scoped resource, apply order is:
	// - First apply ClusterOverridePolicy;
	// - Then apply OverridePolicy;
	ApplyOverridePolicies(rawObj *unstructured.Unstructured, cluster string) (appliedClusterPolicies *AppliedOverrides, appliedNamespacedPolicies *AppliedOverrides, err error)
}

// GeneralOverridePolicy is an abstract object of ClusterOverridePolicy and OverridePolicy
type GeneralOverridePolicy interface {
	// GetName returns the name of OverridePolicy
	GetName() string
	// GetNamespace returns the namespace of OverridePolicy
	GetNamespace() string
	// GetOverrideSpec returns the OverrideSpec of OverridePolicy
	GetOverrideSpec() policyv1alpha1.OverrideSpec
}

// overrideOption define the JSONPatch operator
type overrideOption struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type policyOverriders struct {
	name       string
	namespace  string
	overriders policyv1alpha1.Overriders
}

type overrideManagerImpl struct {
	client karmadaversioned.Interface

	cops []policyv1alpha1.ClusterOverridePolicy
	ops  []policyv1alpha1.OverridePolicy
}

// New builds an OverrideManager instance.
func New(client karmadaversioned.Interface,
	cops []policyv1alpha1.ClusterOverridePolicy,
	ops []policyv1alpha1.OverridePolicy) OverrideManager {
	return &overrideManagerImpl{
		client: client,
		cops:   cops,
		ops:    ops,
	}
}

func (o *overrideManagerImpl) ApplyOverridePolicies(rawObj *unstructured.Unstructured, clusterName string) (*AppliedOverrides, *AppliedOverrides, error) {
	clusterObj, err := o.client.ClusterV1alpha1().Clusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get member cluster: %s, error: %v", clusterName, err)
		return nil, nil, err
	}

	var appliedClusterOverrides *AppliedOverrides
	var appliedNamespacedOverrides *AppliedOverrides

	// Apply cluster scoped override policies
	appliedClusterOverrides, err = o.applyClusterOverrides(rawObj, clusterObj)
	if err != nil {
		klog.Errorf("Failed to apply cluster override policies. error: %v", err)
		return nil, nil, err
	}

	// For namespace scoped resources, should apply override policies under the same namespace.
	// No matter the resources propagated by ClusterPropagationPolicy or PropagationPolicy.
	if len(rawObj.GetNamespace()) > 0 {
		// Apply namespace scoped override policies
		appliedNamespacedOverrides, err = o.applyNamespacedOverrides(rawObj, clusterObj)
		if err != nil {
			klog.Errorf("Failed to apply namespaced override policies. error: %v", err)
			return nil, nil, err
		}
	}
	return appliedClusterOverrides, appliedNamespacedOverrides, nil
}

// applyClusterOverrides will apply overrides according to ClusterOverridePolicy instructions.
func (o *overrideManagerImpl) applyClusterOverrides(rawObj *unstructured.Unstructured, cluster *clusterv1alpha1.Cluster) (*AppliedOverrides, error) {
	// get all cluster-scoped override policies
	policyList, err := o.listClusterOverrides()
	if err != nil {
		klog.Errorf("Failed to list cluster override policies, error: %v", err)
		return nil, err
	}

	if len(policyList.Items) == 0 {
		return nil, nil
	}

	items := make([]GeneralOverridePolicy, 0, len(policyList.Items))
	for i := range policyList.Items {
		items = append(items, &policyList.Items[i])
	}
	matchingPolicyOverriders := o.getOverridersFromOverridePolicies(items, rawObj, cluster)
	if len(matchingPolicyOverriders) == 0 {
		klog.V(4).Infof("No cluster override policy for resource: %s/%s", rawObj.GetNamespace(), rawObj.GetName())
		return nil, nil
	}

	appliedList := &AppliedOverrides{}
	for _, p := range matchingPolicyOverriders {
		if err := applyPolicyOverriders(rawObj, p.overriders); err != nil {
			klog.Errorf("Failed to apply cluster overrides(%s) for resource(%s/%s), error: %v", p.name, rawObj.GetNamespace(), rawObj.GetName(), err)
			return nil, err
		}
		klog.V(4).Infof("Applied cluster overrides(%s) for resource(%s/%s)", p.name, rawObj.GetNamespace(), rawObj.GetName())
		appliedList.Add(p.name, p.overriders)
	}

	return appliedList, nil
}

func (o *overrideManagerImpl) listClusterOverrides() (*policyv1alpha1.ClusterOverridePolicyList, error) {
	// get all cluster-scoped override policies
	policyList, err := o.client.PolicyV1alpha1().ClusterOverridePolicies().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	mapping := make(map[string]policyv1alpha1.ClusterOverridePolicy)
	for _, policy := range policyList.Items {
		mapping[policy.Name] = policy
	}
	for _, policy := range o.cops {
		mapping[policy.Name] = policy
	}
	items := make([]policyv1alpha1.ClusterOverridePolicy, 0, len(mapping))
	for _, policy := range mapping {
		items = append(items, policy)
	}
	policyList.Items = items
	return policyList, nil
}

// applyNamespacedOverrides will apply overrides according to OverridePolicy instructions.
func (o *overrideManagerImpl) applyNamespacedOverrides(rawObj *unstructured.Unstructured, cluster *clusterv1alpha1.Cluster) (*AppliedOverrides, error) {
	// get all namespace-scoped override policies
	policyList, err := o.listNamespacedOverrides(rawObj.GetNamespace())
	if err != nil {
		klog.Errorf("Failed to list override policies from namespace: %s, error: %v", rawObj.GetNamespace(), err)
		return nil, err
	}

	if len(policyList.Items) == 0 {
		return nil, nil
	}

	items := make([]GeneralOverridePolicy, 0, len(policyList.Items))
	for i := range policyList.Items {
		items = append(items, &policyList.Items[i])
	}
	matchingPolicyOverriders := o.getOverridersFromOverridePolicies(items, rawObj, cluster)
	if len(matchingPolicyOverriders) == 0 {
		klog.V(4).Infof("No override policy for the %s resource(%s/%s) on the cluster %s", rawObj.GetKind(), rawObj.GetNamespace(), rawObj.GetName(), cluster.Name)
		return nil, nil
	}

	appliedList := &AppliedOverrides{}
	for _, p := range matchingPolicyOverriders {
		if err := applyPolicyOverriders(rawObj, p.overriders); err != nil {
			klog.Errorf("Failed to apply overrides(%s/%s) for the %s resource(%s/%s) on the cluster %s, error: %v", p.namespace, p.name, rawObj.GetKind(), rawObj.GetNamespace(), rawObj.GetName(), cluster.Name, err)
			return nil, err
		}
		klog.V(4).Infof("Applied overrides(%s/%s) for the %s resource(%s/%s) on the cluster %s ", p.namespace, p.name, rawObj.GetKind(), rawObj.GetNamespace(), rawObj.GetName(), cluster.Name)
		appliedList.Add(p.name, p.overriders)
	}

	return appliedList, nil
}

func (o *overrideManagerImpl) listNamespacedOverrides(namespace string) (*policyv1alpha1.OverridePolicyList, error) {
	// get all namespace-scoped override policies
	policyList, err := o.client.PolicyV1alpha1().OverridePolicies(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	mapping := make(map[string]policyv1alpha1.OverridePolicy)
	for _, policy := range policyList.Items {
		mapping[policy.Name] = policy
	}
	for _, policy := range o.ops {
		if policy.Namespace != namespace {
			continue
		}
		mapping[policy.Name] = policy
	}
	items := make([]policyv1alpha1.OverridePolicy, 0, len(mapping))
	for _, policy := range mapping {
		items = append(items, policy)
	}
	policyList.Items = items
	return policyList, nil
}

func (o *overrideManagerImpl) getOverridersFromOverridePolicies(policies []GeneralOverridePolicy, resource *unstructured.Unstructured, cluster *clusterv1alpha1.Cluster) []policyOverriders {
	resourceMatchingPolicies := make([]GeneralOverridePolicy, 0)
	for _, policy := range policies {
		if len(policy.GetOverrideSpec().ResourceSelectors) == 0 {
			resourceMatchingPolicies = append(resourceMatchingPolicies, policy)
			continue
		}

		if util.ResourceMatchSelectors(resource, policy.GetOverrideSpec().ResourceSelectors...) {
			resourceMatchingPolicies = append(resourceMatchingPolicies, policy)
		}
	}

	clusterMatchingPolicyOverriders := make([]policyOverriders, 0)
	for _, policy := range resourceMatchingPolicies {
		overrideRules := policy.GetOverrideSpec().OverrideRules
		// Since the tuple of '.spec.TargetCluster' and '.spec.Overriders' can not co-exist with '.spec.OverrideRules'
		// (guaranteed by webhook), so we only look '.spec.OverrideRules' here.
		if len(overrideRules) == 0 {
			overrideRules = []policyv1alpha1.RuleWithCluster{
				{
					//nolint:staticcheck
					// disable `deprecation` check for backward compatibility.
					TargetCluster: policy.GetOverrideSpec().TargetCluster,
					//nolint:staticcheck
					// disable `deprecation` check for backward compatibility.
					Overriders: policy.GetOverrideSpec().Overriders,
				},
			}
		}
		for _, rule := range overrideRules {
			if rule.TargetCluster == nil || (rule.TargetCluster != nil && util.ClusterMatches(cluster, *rule.TargetCluster)) {
				clusterMatchingPolicyOverriders = append(clusterMatchingPolicyOverriders, policyOverriders{
					name:       policy.GetName(),
					namespace:  policy.GetNamespace(),
					overriders: rule.Overriders,
				})
			}
		}
	}

	// select policy in which at least one PlaintextOverrider matches target resource.
	// TODO(RainbowMango): check if the overrider instructions can be applied to target resource.

	sort.Slice(clusterMatchingPolicyOverriders, func(i, j int) bool {
		return clusterMatchingPolicyOverriders[i].name < clusterMatchingPolicyOverriders[j].name
	})

	return clusterMatchingPolicyOverriders
}

// applyJSONPatch applies the override on to the given unstructured object.
func applyJSONPatch(obj *unstructured.Unstructured, overrides []overrideOption) error {
	jsonPatchBytes, err := json.Marshal(overrides)
	if err != nil {
		return err
	}

	patch, err := jsonpatch.DecodePatch(jsonPatchBytes)
	if err != nil {
		return err
	}

	objectJSONBytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}

	patchedObjectJSONBytes, err := patch.Apply(objectJSONBytes)
	if err != nil {
		return err
	}

	err = obj.UnmarshalJSON(patchedObjectJSONBytes)
	return err
}

// applyPolicyOverriders applies OverridePolicy/ClusterOverridePolicy overriders to target object
func applyPolicyOverriders(rawObj *unstructured.Unstructured, overriders policyv1alpha1.Overriders) error {
	err := applyImageOverriders(rawObj, overriders.ImageOverrider)
	if err != nil {
		return err
	}
	// patch command
	if err := applyCommandOverriders(rawObj, overriders.CommandOverrider); err != nil {
		return err
	}
	// patch args
	if err := applyArgsOverriders(rawObj, overriders.ArgsOverrider); err != nil {
		return err
	}

	return applyJSONPatch(rawObj, parseJSONPatchesByPlaintext(overriders.Plaintext))
}

func applyImageOverriders(rawObj *unstructured.Unstructured, imageOverriders []policyv1alpha1.ImageOverrider) error {
	for index := range imageOverriders {
		patches, err := buildPatches(rawObj, &imageOverriders[index])
		if err != nil {
			klog.Errorf("Build patches with imageOverrides err: %v", err)
			return err
		}

		klog.V(4).Infof("Parsed JSON patches by imageOverriders(%+v): %+v", imageOverriders[index], patches)
		if err = applyJSONPatch(rawObj, patches); err != nil {
			return err
		}
	}

	return nil
}

func applyCommandOverriders(rawObj *unstructured.Unstructured, commandOverriders []policyv1alpha1.CommandArgsOverrider) error {
	for index := range commandOverriders {
		patches, err := buildCommandArgsPatches(CommandString, rawObj, &commandOverriders[index])
		if err != nil {
			return err
		}

		klog.V(4).Infof("Parsed JSON patches by commandOverriders(%+v): %+v", commandOverriders[index], patches)
		if err = applyJSONPatch(rawObj, patches); err != nil {
			return err
		}
	}

	return nil
}

func applyArgsOverriders(rawObj *unstructured.Unstructured, argsOverriders []policyv1alpha1.CommandArgsOverrider) error {
	for index := range argsOverriders {
		patches, err := buildCommandArgsPatches(ArgsString, rawObj, &argsOverriders[index])
		if err != nil {
			return err
		}

		klog.V(4).Infof("Parsed JSON patches by argsOverriders(%+v): %+v", argsOverriders[index], patches)
		if err = applyJSONPatch(rawObj, patches); err != nil {
			return err
		}
	}

	return nil
}

func parseJSONPatchesByPlaintext(overriders []policyv1alpha1.PlaintextOverrider) []overrideOption {
	patches := make([]overrideOption, 0, len(overriders))
	for i := range overriders {
		patches = append(patches, overrideOption{
			Op:    string(overriders[i].Operator),
			Path:  overriders[i].Path,
			Value: overriders[i].Value,
		})
	}
	return patches
}
