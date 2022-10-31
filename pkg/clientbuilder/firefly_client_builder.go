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

package clientbuilder

import (
	"k8s.io/client-go/dynamic"
	restclient "k8s.io/client-go/rest"
	"k8s.io/controller-manager/pkg/clientbuilder"
	"k8s.io/klog/v2"

	fireflyversioned "github.com/firefly-io/firefly/pkg/generated/clientset/versioned"
)

// FireflyControllerClientBuilder allows you to get clients and configs for firefly controllers
type FireflyControllerClientBuilder interface {
	clientbuilder.ControllerClientBuilder
	DynamicClient(name string) (dynamic.Interface, error)
	DynamicClientOrDie(name string) dynamic.Interface
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

// DynamicClient returns a dynamic.Interface built from the ClientBuilder
func (b SimpleFireflyControllerClientBuilder) DynamicClient(name string) (dynamic.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(clientConfig)
}

// DynamicClientOrDie returns a karmadaversioned.interface built from the ClientBuilder with no error.
// If it gets an error getting the client, it will log the error and kill the process it's running in.
func (b SimpleFireflyControllerClientBuilder) DynamicClientOrDie(name string) dynamic.Interface {
	client, err := b.DynamicClient(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
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
