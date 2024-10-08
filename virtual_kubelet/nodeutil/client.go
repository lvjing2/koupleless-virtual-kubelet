package nodeutil

import (
	"k8s.io/client-go/util/flowcontrol"
	"os"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	coordclientset "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientsetFromEnv returns a kuberentes client set from:
// 1. the passed in kubeconfig path
// 2. If the kubeconfig path is empty or non-existent, then the in-cluster config is used.
func ClientsetFromEnv(kubeConfigPath string) (*kubernetes.Clientset, error) {
	var (
		config *rest.Config
		err    error
	)

	if kubeConfigPath != "" {
		config, err = clientsetFromEnvKubeConfigPath(kubeConfigPath)
	} else {
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, errors.Wrap(err, "error getting rest client config")
	}

	if config == nil {
		return nil, errors.New("error getting rest client config")
	}

	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(1000, 1000)

	return kubernetes.NewForConfig(config)
}

func clientsetFromEnvKubeConfigPath(kubeConfigPath string) (*rest.Config, error) {
	_, err := os.Stat(kubeConfigPath)
	if os.IsNotExist(err) {
		return rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// NodeLeaseV1Client creates a V1 Lease client for use with node leases from the passed in client.
//
// Use this with node.WithNodeEnableLeaseV1Beta1 when creating a node controller.
func NodeLeaseV1Client(client kubernetes.Interface) coordclientset.LeaseInterface {
	return client.CoordinationV1().Leases(corev1.NamespaceNodeLease)
}
