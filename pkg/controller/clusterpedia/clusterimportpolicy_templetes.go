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

const (
	KarmadaClusterImportPolicyTemplate = `apiVersion: policy.clusterpedia.io/v1alpha1
kind: ClusterImportPolicy
metadata:
  name: karmada
spec:
  source:
    group: "cluster.karmada.io"
    versions: []
    resource: clusters
  references:
    - group: ""
      resource: secrets
      namespaceTemplate: "{{ .source.spec.secretRef.namespace }}"
      nameTemplate: "{{ .source.spec.secretRef.name }}"
      key: secret
  nameTemplate: "{{ .source.metadata.name }}"
  template: |
    spec:
      apiserver: "{{ .source.spec.apiEndpoint }}"
      tokenData: "{{ .references.secret.data.token }}"
      caData: "{{ .references.secret.data.caBundle }}"
      syncResources:
        - group: ""
          resources:
            - "pods"
            - "services"
        - group: "apps"
          resources:
            - "*"
      syncResourcesRefName: ""
  creationCondition: |
    {{ if ne .source.spec.apiEndpoint "" }}
      {{ range .source.status.conditions }}
        {{ if eq .type "Ready" }}
          {{ if eq .status "True" }} true {{ end }}
        {{ end }}
      {{ end }}
    {{ end }}`
)
