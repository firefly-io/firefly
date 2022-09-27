package clientbuilder

import (
	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	restclient "k8s.io/client-go/rest"
	"k8s.io/controller-manager/pkg/clientbuilder"
	"k8s.io/klog/v2"
)

// KarmadaControllerClientBuilder allows you to get clients and configs for controllers of the firefly-karmada-manager
type KarmadaControllerClientBuilder interface {
	clientbuilder.ControllerClientBuilder
	KarmadaClient(name string) (karmadaversioned.Interface, error)
	KarmadaClientOrDie(name string) karmadaversioned.Interface
}

// make sure that SimpleKarmadaControllerClientBuilder implements KarmadaControllerClientBuilder
var _ KarmadaControllerClientBuilder = SimpleKarmadaControllerClientBuilder{}

// NewSimpleKarmadaControllerClientBuilder creates a SimpleKarmadaControllerClientBuilder
func NewSimpleKarmadaControllerClientBuilder(config *restclient.Config) SimpleKarmadaControllerClientBuilder {
	return SimpleKarmadaControllerClientBuilder{
		clientbuilder.SimpleControllerClientBuilder{
			ClientConfig: config,
		},
	}
}

// SimpleKarmadaControllerClientBuilder returns a fixed client with different user agents
type SimpleKarmadaControllerClientBuilder struct {
	clientbuilder.SimpleControllerClientBuilder
}

// KarmadaClient returns a karmadaversioned.Interface built from the ClientBuilder
func (b SimpleKarmadaControllerClientBuilder) KarmadaClient(name string) (karmadaversioned.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return karmadaversioned.NewForConfig(clientConfig)
}

// KarmadaClientOrDie returns a karmadaversioned.interface built from the ClientBuilder with no error.
// If it gets an error getting the client, it will log the error and kill the process it's running in.
func (b SimpleKarmadaControllerClientBuilder) KarmadaClientOrDie(name string) karmadaversioned.Interface {
	client, err := b.KarmadaClient(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
}
