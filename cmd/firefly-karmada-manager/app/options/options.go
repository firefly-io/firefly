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

package options

import (
	"fmt"
	"net"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	clientset "k8s.io/client-go/kubernetes"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/controller-manager/config"
	cmoptions "k8s.io/controller-manager/options"
	netutils "k8s.io/utils/net"

	fireflycontrollerconfig "github.com/firefly-io/firefly/cmd/firefly-karmada-manager/app/config"
	fireflyctrlmgrconfig "github.com/firefly-io/firefly/pkg/controller/apis/config"
)

const (
	// FireflyKarmadaManagerUserAgent is the userAgent name when starting firefly-controller managers.
	FireflyKarmadaManagerUserAgent = "firefly-karmada-manager"
)

// FireflyControllerManagerOptions is the main context object for the firefly-controller-manager.
type FireflyControllerManagerOptions struct {
	Generic *cmoptions.GenericControllerManagerConfigurationOptions

	SecureServing  *apiserveroptions.SecureServingOptionsWithLoopback
	Authentication *apiserveroptions.DelegatingAuthenticationOptions
	Authorization  *apiserveroptions.DelegatingAuthorizationOptions
	Metrics        *metrics.Options
	Logs           *logs.Options

	KarmadaMaster      string
	KarmadaKubeconfig  string
	FireflyKubeconfig  string
	EstimatorNamespace string
	KarmadaName        string
}

// NewFireflyControllerManagerOptions creates a new FireflyControllerManagerOptions with a default config.
func NewFireflyControllerManagerOptions() (*FireflyControllerManagerOptions, error) {
	componentConfig, err := NewDefaultComponentConfig()
	if err != nil {
		return nil, err
	}

	s := FireflyControllerManagerOptions{
		Generic: cmoptions.NewGenericControllerManagerConfigurationOptions(&componentConfig.Generic),

		SecureServing:  apiserveroptions.NewSecureServingOptions().WithLoopback(),
		Authentication: apiserveroptions.NewDelegatingAuthenticationOptions(),
		Authorization:  apiserveroptions.NewDelegatingAuthorizationOptions(),
		Metrics:        metrics.NewOptions(),
		Logs:           logs.NewOptions(),
	}

	// Set the PairName but leave certificate directory blank to generate in-memory by default
	s.SecureServing.ServerCert.CertDirectory = ""
	s.SecureServing.ServerCert.PairName = "firefly-karmada-manager"
	// s.SecureServing.BindPort = 10257
	s.SecureServing.BindPort = 10357

	s.Generic.LeaderElection.ResourceName = "firefly-karmada-manager"
	s.Generic.LeaderElection.ResourceNamespace = "firefly-system"
	return &s, nil
}

func NewDefaultComponentConfig() (fireflyctrlmgrconfig.FireflyControllerManagerConfiguration, error) {
	internal := fireflyctrlmgrconfig.FireflyControllerManagerConfiguration{
		Generic: config.GenericControllerManagerConfiguration{
			Address:                 "0.0.0.0",
			Controllers:             []string{"*"},
			MinResyncPeriod:         metav1.Duration{Duration: 12 * time.Hour},
			ControllerStartInterval: metav1.Duration{Duration: 0 * time.Second},
		},
	}
	return internal, nil
}

// Flags returns flags for a specific APIServer by section name
func (s *FireflyControllerManagerOptions) Flags(allControllers []string, disabledByDefaultControllers []string) cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	s.Generic.AddFlags(&fss, allControllers, disabledByDefaultControllers)

	s.SecureServing.AddFlags(fss.FlagSet("secure serving"))
	s.Authentication.AddFlags(fss.FlagSet("authentication"))
	s.Authorization.AddFlags(fss.FlagSet("authorization"))

	s.Metrics.AddFlags(fss.FlagSet("metrics"))
	logsapi.AddFlags(s.Logs, fss.FlagSet("logs"))

	fs := fss.FlagSet("misc")
	fs.StringVar(&s.KarmadaMaster, "karmada-master", s.KarmadaMaster, "The address of the karmada API server (overrides any value in karmada-kubeconfig).")
	fs.StringVar(&s.KarmadaKubeconfig, "karmada-kubeconfig", s.KarmadaKubeconfig, "Path to karmada kubeconfig file with authorization and master location information.")
	fs.StringVar(&s.FireflyKubeconfig, "firefly-kubeconfig", s.FireflyKubeconfig, "Path to firefly kubeconfig file with authorization and master location information.")
	fs.StringVarP(&s.EstimatorNamespace, "estimator-namespace", "n", os.Getenv("ESTIMATOR_NAMESPACE"), "It represents the namespace which scheduler-estimator will be deployed. It should be the same as the namespace of a firefly karmada.")
	fs.StringVar(&s.KarmadaName, "karmada-name", s.KarmadaName, "It represents the name of a firefly karmada object.")

	return fss
}

// ApplyTo fills up controller manager config with options.
func (s *FireflyControllerManagerOptions) ApplyTo(c *fireflycontrollerconfig.Config) error {
	if err := s.Generic.ApplyTo(&c.ComponentConfig.Generic); err != nil {
		return err
	}
	if err := s.SecureServing.ApplyTo(&c.SecureServing, &c.LoopbackClientConfig); err != nil {
		return err
	}
	if s.SecureServing.BindPort != 0 || s.SecureServing.Listener != nil {
		if err := s.Authentication.ApplyTo(&c.Authentication, c.SecureServing, nil); err != nil {
			return err
		}
		if err := s.Authorization.ApplyTo(&c.Authorization); err != nil {
			return err
		}
	}
	return nil
}

// Validate is used to validate the options and config before launching the controller manager
func (s *FireflyControllerManagerOptions) Validate(allControllers []string, disabledByDefaultControllers []string) error {
	var errs []error
	return utilerrors.NewAggregate(errs)
}

// Config return a controller manager config objective
func (s FireflyControllerManagerOptions) Config(allControllers []string, disabledByDefaultControllers []string) (*fireflycontrollerconfig.Config, error) {
	if err := s.Validate(allControllers, disabledByDefaultControllers); err != nil {
		return nil, err
	}

	if err := s.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	karmadaKubeconfig, err := clientcmd.BuildConfigFromFlags(s.KarmadaMaster, s.KarmadaKubeconfig)
	if err != nil {
		return nil, err
	}
	karmadaKubeconfig.DisableCompression = true
	karmadaKubeconfig.ContentConfig.AcceptContentTypes = s.Generic.ClientConnection.AcceptContentTypes
	karmadaKubeconfig.ContentConfig.ContentType = s.Generic.ClientConnection.ContentType
	karmadaKubeconfig.QPS = s.Generic.ClientConnection.QPS
	karmadaKubeconfig.Burst = int(s.Generic.ClientConnection.Burst)

	karmadaKubeClient, err := clientset.NewForConfig(restclient.AddUserAgent(karmadaKubeconfig, FireflyKarmadaManagerUserAgent))
	if err != nil {
		return nil, err
	}

	fireflyKubeconfig, err := clientcmd.BuildConfigFromFlags("", s.FireflyKubeconfig)
	if err != nil {
		return nil, err
	}
	fireflyKubeconfig.DisableCompression = true
	fireflyKubeconfig.ContentConfig.AcceptContentTypes = s.Generic.ClientConnection.AcceptContentTypes
	fireflyKubeconfig.ContentConfig.ContentType = s.Generic.ClientConnection.ContentType
	fireflyKubeconfig.QPS = s.Generic.ClientConnection.QPS
	fireflyKubeconfig.Burst = int(s.Generic.ClientConnection.Burst)
	fireflyKubeClient, err := clientset.NewForConfig(restclient.AddUserAgent(fireflyKubeconfig, FireflyKarmadaManagerUserAgent))
	if err != nil {
		return nil, err
	}

	eventBroadcaster := record.NewBroadcaster()
	eventRecorder := eventBroadcaster.NewRecorder(clientgokubescheme.Scheme, v1.EventSource{Component: FireflyKarmadaManagerUserAgent})

	c := &fireflycontrollerconfig.Config{
		KarmadaKubeClient:  karmadaKubeClient,
		KarmadaKubeconfig:  karmadaKubeconfig,
		FireflyKubeClient:  fireflyKubeClient,
		FireflyKubeconfig:  fireflyKubeconfig,
		EventBroadcaster:   eventBroadcaster,
		EventRecorder:      eventRecorder,
		EstimatorNamespace: s.EstimatorNamespace,
		KarmadaName:        s.KarmadaName,
	}
	if err := s.ApplyTo(c); err != nil {
		return nil, err
	}
	s.Metrics.Apply()

	return c, nil
}
