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
	"io/ioutil"
	"net"
	"net/http"

	netutils "k8s.io/utils/net"
)

const (
	// Query the IP address of the current host accessing the Internet
	getInternetIPUrl = "https://myexternalip.com/raw"
)

// InternetIP Current host Internet IP.
func InternetIP() (net.IP, error) {
	resp, err := http.Get(getInternetIPUrl)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return netutils.ParseIPSloppy(string(content)), nil
}
