package clusterpedia

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestA(t *testing.T) {
	obj := &unstructured.Unstructured{}
	err := yaml.Unmarshal([]byte(KarmadaClusterImportPolicyTemplate), obj)
	// yaml.Unmarshal()
	// data, err := yaml.Marshal(KarmadaClusterImportPolicyTemplate)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// data1, err := yaml.YAMLToJSON([]byte(KarmadaClusterImportPolicyTemplate))
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal(*obj)
	// t.Log(string(data))
	// if err := obj.UnmarshalJSON(data1); err != nil {
	// 	t.Fatal(err)
	// }
}
