package karmada

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
)

func (ctrl *KarmadaController) EnsureKarmadaCRDs(karmada *installv1alpha1.Karmada) error {
	clientConfig, err := ctrl.GenerateClientConfig(karmada)
	if err != nil {
		return err
	}
	result := resource.NewBuilder(clientGetter{restConfig: clientConfig}).
		Unstructured().
		FilenameParam(false, &resource.FilenameOptions{Recursive: false, Filenames: []string{"./pkg/controller/karmada/crds"}}).
		Flatten().Do()
	return result.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		_, err1 := resource.NewHelper(info.Client, info.Mapping).Create(info.Namespace, true, info.Object)
		if err1 != nil {
			if !errors.IsAlreadyExists(err1) {
				return err1
			}
		}
		return nil
	})
}

type clientGetter struct {
	restConfig *rest.Config
}

func (c clientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.restConfig, nil
}
func (c clientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(c.restConfig)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(discoveryClient), nil
}

func (c clientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}
