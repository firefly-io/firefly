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

package app

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/karmada-io/karmada/pkg/util/gclient"
	"github.com/spf13/cobra"
	apiserver "k8s.io/apiserver/pkg/server"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/term"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/carlory/firefly/cmd/firefly-webhook/app/options"
	"github.com/carlory/firefly/pkg/karmada/webhook/work"
)

// NewWebhookCommand creates a *cobra.Command object with default parameters
func NewWebhookCommand() *cobra.Command {
	opts := options.NewOptions()
	cmd := &cobra.Command{
		Use:  "karmada-webhook",
		Long: `Start a karmada webhook server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Run(opts)
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}

	fs := cmd.Flags()
	namedFlagSets := cliflag.NamedFlagSets{}
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name(), logs.SkipLoggingConfigurationFlags())

	genericFlagSet := namedFlagSets.FlagSet("generic")
	genericFlagSet.AddGoFlagSet(flag.CommandLine)
	opts.AddFlags(genericFlagSet)

	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, namedFlagSets, cols)
	return cmd
}

// Run runs the specified KarmadaWebhookServer.  This should never exit.
func Run(opts *options.Options) error {
	// To help debugging, immediately log version
	klog.Infof("Version: %+v", version.Get())

	klog.InfoS("Golang settings", "GOGC", os.Getenv("GOGC"), "GOMAXPROCS", os.Getenv("GOMAXPROCS"), "GOTRACEBACK", os.Getenv("GOTRACEBACK"))

	config, err := controllerruntime.GetConfig()
	if err != nil {
		panic(err)
	}
	config.QPS, config.Burst = opts.KubeAPIQPS, opts.KubeAPIBurst

	hookManager, err := controllerruntime.NewManager(config, controllerruntime.Options{
		Logger: klog.Background(),
		Scheme: gclient.NewSchema(),
		WebhookServer: &webhook.Server{
			Host:          opts.BindAddress,
			Port:          opts.SecurePort,
			CertDir:       opts.CertDir,
			CertName:      opts.CertName,
			KeyName:       opts.KeyName,
			TLSMinVersion: opts.TLSMinVersion,
		},
		LeaderElection:         false,
		MetricsBindAddress:     opts.MetricsBindAddress,
		HealthProbeBindAddress: opts.HealthProbeBindAddress,
	})
	if err != nil {
		klog.Errorf("failed to build webhook server: %v", err)
		return err
	}

	klog.Info("registering webhooks to the webhook server")
	hookServer := hookManager.GetWebhookServer()
	hookServer.Register("/mutate-work-karmada-io-v1alpha1-work", &webhook.Admission{Handler: &work.MutatingAdmission{
		Client: hookManager.GetClient(),
	}})
	hookServer.WebhookMux.Handle("/readyz/", http.StripPrefix("/readyz/", &healthz.Handler{}))

	// blocks until the context is done.
	if err := hookManager.Start(apiserver.SetupSignalContext()); err != nil {
		klog.Errorf("webhook server exits unexpectedly: %v", err)
		return err
	}

	// never reach here
	return nil
}
