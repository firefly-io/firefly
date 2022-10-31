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

package util

import (
	"context"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/firefly-io/firefly/pkg/constants"
)

// Waiter is an interface for waiting for criteria in Kubernetes to happen
type Waiter interface {
	// WaitForKubeAPI waits for the API Server's /healthz endpoint to become "ok"
	WaitForKubeAPI() error
	// WaitForPodsWithLabel waits for Pods in a given namespace to become Ready
	WaitForPodsWithLabel(namespace, kvLabel string) error
	// // WaitForHealthyKubelet blocks until the kubelet /healthz endpoint returns 'ok'
	// WaitForHealthyKubelet(initialTimeout time.Duration, healthzEndpoint string) error
	// // WaitForKubeletAndFunc is a wrapper for WaitForHealthyKubelet that also blocks for a function
	// WaitForKubeletAndFunc(f func() error) error
	// SetTimeout adjusts the timeout to the specified duration
	SetTimeout(timeout time.Duration)
}

// KubeWaiter is an implementation of Waiter that is backed by a Kubernetes client
type KubeWaiter struct {
	client  clientset.Interface
	timeout time.Duration
}

// NewKubeWaiter returns a new Waiter object that talks to the given Kubernetes cluster
func NewKubeWaiter(client clientset.Interface, timeout time.Duration) Waiter {
	return &KubeWaiter{
		client:  client,
		timeout: timeout,
	}
}

// WaitForKubeAPI waits for the API Server's /healthz endpoint to report "ok"
func (w *KubeWaiter) WaitForKubeAPI() error {
	start := time.Now()
	return wait.PollImmediate(constants.APICallRetryInterval, w.timeout, func() (bool, error) {
		healthStatus := 0
		w.client.Discovery().RESTClient().Get().AbsPath("/healthz").Do(context.TODO()).StatusCode(&healthStatus)
		if healthStatus != http.StatusOK {
			return false, nil
		}

		klog.InfoS("All control plane components are healthy after a few seconds", "duration", time.Since(start).Seconds())
		return true, nil
	})
}

// WaitForPodsWithLabel will lookup pods with the given label and wait until they are all
// reporting status as running.
func (w *KubeWaiter) WaitForPodsWithLabel(namespace, kvLabel string) error {

	lastKnownPodNumber := -1
	return wait.PollImmediate(constants.APICallRetryInterval, w.timeout, func() (bool, error) {
		listOpts := metav1.ListOptions{LabelSelector: kvLabel}
		pods, err := w.client.CoreV1().Pods(namespace).List(context.TODO(), listOpts)
		if err != nil {
			klog.ErrorS(err, "Error getting Pods with label selector", "labelSelector", kvLabel)
			return false, nil
		}

		if lastKnownPodNumber != len(pods.Items) {
			klog.InfoS("Found pods for label selector ", "namespace", namespace, "podCount", len(pods.Items), "labelSelector", kvLabel)
			lastKnownPodNumber = len(pods.Items)
		}

		if len(pods.Items) == 0 {
			return false, nil
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase != v1.PodRunning {
				return false, nil
			}
		}

		return true, nil
	})
}

// WaitForHealthyKubelet blocks until the kubelet /healthz endpoint returns 'ok'
// func (w *KubeWaiter) WaitForHealthyKubelet(initialTimeout time.Duration, healthzEndpoint string) error {
// 	time.Sleep(initialTimeout)
// 	fmt.Printf("[kubelet-check] Initial timeout of %v passed.\n", initialTimeout)
// 	return TryRunCommand(func() error {
// 		client := &http.Client{Transport: netutil.SetOldTransportDefaults(&http.Transport{})}
// 		resp, err := client.Get(healthzEndpoint)
// 		if err != nil {
// 			fmt.Println("[kubelet-check] It seems like the kubelet isn't running or healthy.")
// 			fmt.Printf("[kubelet-check] The HTTP call equal to 'curl -sSL %s' failed with error: %v.\n", healthzEndpoint, err)
// 			return err
// 		}
// 		defer resp.Body.Close()
// 		if resp.StatusCode != http.StatusOK {
// 			fmt.Println("[kubelet-check] It seems like the kubelet isn't running or healthy.")
// 			fmt.Printf("[kubelet-check] The HTTP call equal to 'curl -sSL %s' returned HTTP code %d\n", healthzEndpoint, resp.StatusCode)
// 			return errors.New("the kubelet healthz endpoint is unhealthy")
// 		}
// 		return nil
// 	}, 5) // a failureThreshold of five means waiting for a total of 155 seconds
// }

// WaitForKubeletAndFunc waits primarily for the function f to execute, even though it might take some time. If that takes a long time, and the kubelet
// /healthz continuously are unhealthy, kubeadm will error out after a period of exponential backoff
// func (w *KubeWaiter) WaitForKubeletAndFunc(f func() error) error {
// 	errorChan := make(chan error, 1)

// 	go func(errC chan error, waiter Waiter) {
// 		if err := waiter.WaitForHealthyKubelet(40*time.Second, fmt.Sprintf("http://localhost:%d/healthz", kubeadmconstants.KubeletHealthzPort)); err != nil {
// 			errC <- err
// 		}
// 	}(errorChan, w)

// 	go func(errC chan error) {
// 		// This main goroutine sends whatever the f function returns (error or not) to the channel
// 		// This in order to continue on success (nil error), or just fail if the function returns an error
// 		errC <- f()
// 	}(errorChan)

// 	// This call is blocking until one of the goroutines sends to errorChan
// 	return <-errorChan
// }

// SetTimeout adjusts the timeout to the specified duration
func (w *KubeWaiter) SetTimeout(timeout time.Duration) {
	w.timeout = timeout
}

// TryRunCommand runs a function a maximum of failureThreshold times, and retries on error. If failureThreshold is hit; the last error is returned
func TryRunCommand(f func() error, failureThreshold int) error {
	backoff := wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   2, // double the timeout for every failure
		Steps:    failureThreshold,
	}
	return wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := f()
		if err != nil {
			// Retry until the timeout
			return false, nil
		}
		// The last f() call was a success, return cleanly
		return true, nil
	})
}
