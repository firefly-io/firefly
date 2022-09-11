package constants

import "time"

const (
	// APICallRetryInterval defines how long kubeadm should wait before retrying a failed API operation
	APICallRetryInterval = 500 * time.Millisecond

	// KarmadaSystemNamespace defines
	KarmadaSystemNamespace = "karmada-system"
)
