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
	"github.com/MakeNowJust/heredoc"
	policyapi "github.com/clusterpedia-io/api/policy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	installv1alpha1 "github.com/firefly-io/firefly/pkg/apis/install/v1alpha1"
)

func (ctrl *ClusterpediaController) GenerateClusterImportPolicyForKamada(clusterpedia *installv1alpha1.Clusterpedia) *policyapi.ClusterImportPolicy {
	syncResources := clusterpedia.Spec.ControlplaneProvider.SyncResources
	syncAllCustomResources := clusterpedia.Spec.ControlplaneProvider.SyncAllCustomResources
	tmpl := map[string]map[string]interface{}{
		"spec": {
			"apiserver":              "{{ .source.spec.apiEndpoint }}",
			"tokenData":              "{{ .references.secret.data.token }}",
			"caData":                 "{{ .references.secret.data.caBundle }}",
			"syncAllCustomResources": syncAllCustomResources,
			"syncResources":          syncResources,
			"syncResourcesRefName":   "",
		},
	}
	tmplData, _ := yaml.Marshal(tmpl)
	policy := &policyapi.ClusterImportPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.clusterpedia.io/v1alpha1",
			Kind:       "ClusterImportPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "karmada",
		},
		Spec: policyapi.ClusterImportPolicySpec{
			Source: policyapi.SourceType{
				Group:    "cluster.karmada.io",
				Versions: []string{},
				Resource: "clusters",
			},
			References: []policyapi.IntendReferenceResourceTemplate{
				{
					BaseReferenceResourceTemplate: policyapi.BaseReferenceResourceTemplate{
						Key:               "secret",
						Group:             "",
						Resource:          "secrets",
						NamespaceTemplate: "{{ .source.spec.secretRef.namespace }}",
						NameTemplate:      "{{ .source.spec.secretRef.name }}",
					},
				},
			},
			NameTemplate: "{{ .source.metadata.name }}",
			Policy: policyapi.Policy{
				Template: string(tmplData),
				CreationCondition: heredoc.Doc(`
          {{ if ne .source.spec.apiEndpoint "" }}
             {{ range .source.status.conditions }}
                {{ if eq .type "Ready" }}
                   {{ if eq .status "True" }} true {{ end }}
                {{ end }}
            {{ end }}
          {{ end }}`),
			},
		},
	}
	return policy
}
