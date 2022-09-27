package clientbuilder

import (
	restclient "k8s.io/client-go/rest"
	"k8s.io/controller-manager/pkg/clientbuilder"
	"k8s.io/klog/v2"

	fireflyversioned "github.com/carlory/firefly/pkg/generated/clientset/versioned"
)

// FireflyControllerClientBuilder allows you to get clients and configs for firefly controllers
type FireflyControllerClientBuilder interface {
	clientbuilder.ControllerClientBuilder
	FireflyClient(name string) (fireflyversioned.Interface, error)
	FireflyClientOrDie(name string) fireflyversioned.Interface
}

// make sure that SimpleFireflyControllerClientBuilder implements FireflyControllerClientBuilder
var _ FireflyControllerClientBuilder = SimpleFireflyControllerClientBuilder{}

// NewSimpleFireflyControllerClientBuilder creates a SimpleFireflyControllerClientBuilder
func NewSimpleFireflyControllerClientBuilder(config *restclient.Config) SimpleFireflyControllerClientBuilder {
	return SimpleFireflyControllerClientBuilder{
		clientbuilder.SimpleControllerClientBuilder{
			ClientConfig: config,
		},
	}
}

// SimpleFireflyControllerClientBuilder returns a fixed client with different user agents
type SimpleFireflyControllerClientBuilder struct {
	clientbuilder.SimpleControllerClientBuilder
}

// FireflyClient returns a fireflyversioned.Interface built from the ClientBuilder
func (b SimpleFireflyControllerClientBuilder) FireflyClient(name string) (fireflyversioned.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return fireflyversioned.NewForConfig(clientConfig)
}

// FireflyClientOrDie returns a fireflyversioned.interface built from the ClientBuilder with no error.
// If it gets an error getting the client, it will log the error and kill the process it's running in.
func (b SimpleFireflyControllerClientBuilder) FireflyClientOrDie(name string) fireflyversioned.Interface {
	client, err := b.FireflyClient(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
}
