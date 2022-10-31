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
	karmadaversioned "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	"k8s.io/client-go/dynamic"
	restclient "k8s.io/client-go/rest"
	"k8s.io/controller-manager/pkg/clientbuilder"
	"k8s.io/klog/v2"

	fireflyclient "github.com/firefly-io/firefly/pkg/karmada/generated/clientset/versioned"
)

// KarmadaControllerClientBuilder allows you to get clients and configs for controllers of the firefly-karmada-manager
type KarmadaControllerClientBuilder interface {
	clientbuilder.ControllerClientBuilder
	DynamicClient(name string) (dynamic.Interface, error)
	DynamicClientOrDie(name string) dynamic.Interface
	KarmadaClient(name string) (karmadaversioned.Interface, error)
	KarmadaClientOrDie(name string) karmadaversioned.Interface
	KarmadaFireflyClient(name string) (fireflyclient.Interface, error)
	KarmadaFireflyClientOrDie(name string) fireflyclient.Interface
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

// DynamicClient returns a dynamic.Interface built from the ClientBuilder
func (b SimpleKarmadaControllerClientBuilder) DynamicClient(name string) (dynamic.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(clientConfig)
}

// DynamicClientOrDie returns a karmadaversioned.interface built from the ClientBuilder with no error.
// If it gets an error getting the client, it will log the error and kill the process it's running in.
func (b SimpleKarmadaControllerClientBuilder) DynamicClientOrDie(name string) dynamic.Interface {
	client, err := b.DynamicClient(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
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

// KarmadaFireflyClient returns a fireflyclient.Interface built from the ClientBuilder
func (b SimpleKarmadaControllerClientBuilder) KarmadaFireflyClient(name string) (fireflyclient.Interface, error) {
	clientConfig, err := b.Config(name)
	if err != nil {
		return nil, err
	}
	return fireflyclient.NewForConfig(clientConfig)
}

// KarmadaFireflyClientOrDie returns a fireflyclient.interface built from the ClientBuilder with no error.
// If it gets an error getting the client, it will log the error and kill the process it's running in.
func (b SimpleKarmadaControllerClientBuilder) KarmadaFireflyClientOrDie(name string) fireflyclient.Interface {
	client, err := b.KarmadaFireflyClient(name)
	if err != nil {
		klog.Fatal(err)
	}
	return client
}
