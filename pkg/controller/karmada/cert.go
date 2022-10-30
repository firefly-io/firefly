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

package karmada

import (
	"context"
	"fmt"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	installv1alpha1 "github.com/carlory/firefly/pkg/apis/install/v1alpha1"
	"github.com/carlory/firefly/pkg/constants"
	"github.com/carlory/firefly/pkg/scheme"
	"github.com/carlory/firefly/pkg/util"
	"github.com/carlory/firefly/pkg/util/certs"
)

var certList = []string{
	"ca",
	"etcd-ca",
	"etcd-server",
	"etcd-client",
	"karmada",
	"apiserver",
	"front-proxy-ca",
	"front-proxy-client",
}

func (ctrl *KarmadaController) genCerts(karmada *installv1alpha1.Karmada, karmadaAPIServerIP []net.IP) error {
	notAfter := time.Now().Add(certs.Duration365d).UTC()

	var etcdServerCertDNS = []string{
		"localhost",
		fmt.Sprintf("%s.%s.svc", constants.KarmadaComponentEtcd, karmada.Namespace),
		fmt.Sprintf("%s.%s.svc.%s", constants.KarmadaComponentEtcd, karmada.Namespace, karmada.Spec.Networking.DNSDomain),
	}
	for number := int32(0); number < 1; number++ {
		etcdServerCertDNS = append(etcdServerCertDNS, fmt.Sprintf("%s-%v.%s.%s.svc", constants.KarmadaComponentEtcd, number, constants.KarmadaComponentEtcd, karmada.Namespace))
		etcdServerCertDNS = append(etcdServerCertDNS, fmt.Sprintf("%s-%v.%s.%s.svc.%s", constants.KarmadaComponentEtcd, number, constants.KarmadaComponentEtcd, karmada.Namespace, karmada.Spec.Networking.DNSDomain))
	}

	etcdServerAltNames := certutil.AltNames{
		DNSNames: etcdServerCertDNS,
		IPs:      []net.IP{netutils.ParseIPSloppy("127.0.0.1")},
	}
	etcdServerCertConfig := certs.NewCertConfig("karmada-etcd-server", []string{}, etcdServerAltNames, &notAfter)
	etcdClientCertCfg := certs.NewCertConfig("karmada-etcd-client", []string{}, certutil.AltNames{}, &notAfter)

	var karmadaDNS = []string{
		"localhost",
		"kubernetes",
		"kubernetes.default",
		"kubernetes.default.svc",
		constants.KarmadaComponentKubeAPIServer,
		constants.KarmadaComponentWebhook,
		constants.FireflyComponentKarmadaWebhook,
		constants.KarmadaComponentAggregratedAPIServer,
		fmt.Sprintf("%s.%s.svc", constants.KarmadaComponentKubeAPIServer, karmada.Namespace),
		fmt.Sprintf("%s.%s.svc.%s", constants.KarmadaComponentKubeAPIServer, karmada.Namespace, karmada.Spec.Networking.DNSDomain),
		fmt.Sprintf("%s.%s.svc.%s", constants.KarmadaComponentWebhook, karmada.Namespace, karmada.Spec.Networking.DNSDomain),
		fmt.Sprintf("%s.%s.svc.%s", constants.FireflyComponentKarmadaWebhook, karmada.Namespace, karmada.Spec.Networking.DNSDomain),
		fmt.Sprintf("%s.%s.svc", constants.KarmadaComponentWebhook, karmada.Namespace),
		fmt.Sprintf("%s.%s.svc", constants.FireflyComponentKarmadaWebhook, karmada.Namespace),
		fmt.Sprintf("%s.%s.svc.%s", constants.KarmadaComponentAggregratedAPIServer, karmada.Namespace, karmada.Spec.Networking.DNSDomain),
		fmt.Sprintf("*.%s.svc.%s", karmada.Namespace, karmada.Spec.Networking.DNSDomain),
		fmt.Sprintf("*.%s.svc", karmada.Namespace),
	}

	karmadaIPs := []net.IP{}
	karmadaIPs = append(
		karmadaIPs,
		netutils.ParseIPSloppy("127.0.0.1"),
		netutils.ParseIPSloppy("10.254.0.1"),
	)
	if len(karmadaAPIServerIP) > 0 {
		karmadaIPs = append(karmadaIPs, karmadaAPIServerIP...)
	}

	internetIP, err := util.InternetIP()
	if err != nil {
		klog.Warningln("Failed to obtain internet IP. ", err)
	} else {
		karmadaIPs = append(karmadaIPs, internetIP)
	}

	karmadaAltNames := certutil.AltNames{
		DNSNames: karmadaDNS,
		IPs:      karmadaIPs,
	}
	karmadaCertCfg := certs.NewCertConfig("system:admin", []string{"system:masters"}, karmadaAltNames, &notAfter)

	apiserverCertCfg := certs.NewCertConfig("karmada-apiserver", []string{""}, karmadaAltNames, &notAfter)

	frontProxyClientCertCfg := certs.NewCertConfig("front-proxy-client", []string{}, certutil.AltNames{}, &notAfter)
	data, err := certs.GenCerts(etcdServerCertConfig, etcdClientCertCfg, karmadaCertCfg, apiserverCertCfg, frontProxyClientCertCfg)
	if err != nil {
		return err
	}

	// Create kubeconfig Secret
	karmadaServerURL := fmt.Sprintf("https://%s.%s.svc.%s:%v", constants.KarmadaComponentKubeAPIServer, karmada.Namespace, karmada.Spec.Networking.DNSDomain, 5443)
	config := certs.CreateWithCerts(karmadaServerURL, "karmada-admin", "karmada-admin", data["ca.crt"], data["karmada.key"], data["karmada.crt"])
	configBytes, err := clientcmd.Write(*config)
	if err != nil {
		return fmt.Errorf("failure while serializing admin kubeConfig. %v", err)
	}

	kubeConfigSecret := SecretFromSpec(karmada.Namespace, "karmada-kubeconfig", corev1.SecretTypeOpaque, map[string]string{"kubeconfig": string(configBytes)})
	controllerutil.SetOwnerReference(karmada, kubeConfigSecret, scheme.Scheme)
	_, err = ctrl.client.CoreV1().Secrets(karmada.Namespace).Create(context.TODO(), kubeConfigSecret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	// Create certs Secret
	etcdCert := map[string]string{
		"etcd-ca.crt":     string(data["etcd-ca.crt"]),
		"etcd-ca.key":     string(data["etcd-ca.key"]),
		"etcd-server.crt": string(data["etcd-server.crt"]),
		"etcd-server.key": string(data["etcd-server.key"]),
	}
	etcdSecret := SecretFromSpec(karmada.Namespace, fmt.Sprintf("%s-cert", constants.KarmadaComponentEtcd), corev1.SecretTypeOpaque, etcdCert)
	controllerutil.SetOwnerReference(karmada, etcdSecret, scheme.Scheme)
	_, err = ctrl.client.CoreV1().Secrets(karmada.Namespace).Create(context.TODO(), etcdSecret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	karmadaCert := map[string]string{}
	for _, v := range certList {
		karmadaCert[fmt.Sprintf("%s.crt", v)] = string(data[fmt.Sprintf("%s.crt", v)])
		karmadaCert[fmt.Sprintf("%s.key", v)] = string(data[fmt.Sprintf("%s.key", v)])
	}
	karmadaSecret := SecretFromSpec(karmada.Namespace, "karmada-cert", corev1.SecretTypeOpaque, karmadaCert)
	controllerutil.SetOwnerReference(karmada, karmadaSecret, scheme.Scheme)
	_, err = ctrl.client.CoreV1().Secrets(karmada.Namespace).Create(context.TODO(), karmadaSecret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	karmadaWebhookCert := map[string]string{
		"tls.crt": string(data["karmada.crt"]),
		"tls.key": string(data["karmada.key"]),
	}
	karmadaWebhookSecret := SecretFromSpec(karmada.Namespace, fmt.Sprintf("%s-cert", constants.KarmadaComponentWebhook), corev1.SecretTypeOpaque, karmadaWebhookCert)
	controllerutil.SetOwnerReference(karmada, karmadaWebhookSecret, scheme.Scheme)
	_, err = ctrl.client.CoreV1().Secrets(karmada.Namespace).Create(context.TODO(), karmadaWebhookSecret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	fireflyKarmadaCert := map[string]string{
		"tls.crt": string(data["karmada.crt"]),
		"tls.key": string(data["karmada.key"]),
	}
	fireflyKarmadaWebhookSecret := SecretFromSpec(karmada.Namespace, fmt.Sprintf("%s-cert", constants.FireflyComponentKarmadaWebhook), corev1.SecretTypeOpaque, fireflyKarmadaCert)
	controllerutil.SetOwnerReference(karmada, fireflyKarmadaWebhookSecret, scheme.Scheme)
	_, err = ctrl.client.CoreV1().Secrets(karmada.Namespace).Create(context.TODO(), fireflyKarmadaWebhookSecret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func SecretFromSpec(namespace, name string, secretType corev1.SecretType, data map[string]string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"karmada.io/bootstrapping": "secret-defaults"},
		},
		//Immutable:  immutable,
		Type:       secretType,
		StringData: data,
	}
}
