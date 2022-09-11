package karmada

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (ctrl *KarmadaController) CreateService(svc *corev1.Service) error {
	_, err := ctrl.client.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func ComponentName(component, name string) string {
	return fmt.Sprintf("%s-%s", name, component)
}

func ComponentImageName(repository, component, version string) string {
	return fmt.Sprintf("%s/%s:%s", repository, component, version)
}
