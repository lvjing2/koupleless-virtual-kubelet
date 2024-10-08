package pod_provider

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_client"
	"github.com/koupleless/virtual-kubelet/tunnel/mqtt_tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func init() {
	mqtt.DefaultMqttClientInitFunc = mqtt_client.NewMockMqttClient
}

func TestBaseProvider_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := &mqtt_tunnel.MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", nil, nil, nil)
	assert.NoError(t, err)
	kubeClient := fake.NewSimpleClientset()

	provider := NewBaseProvider("default", "127.0.0.1", "test_node", kubeClient, mt)
	provider.NotifyPods(ctx, func(pod *corev1.Pod) {
		return
	})

	provider.Run(ctx)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "0.0.1",
						},
					},
				},
			},
		},
	}

	podStatus, err := provider.GetPodStatus(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.Equal(t, podStatus.Phase, corev1.PodSucceeded)

	err = provider.CreatePod(ctx, pod)
	assert.NoError(t, err)

	podLocal, err := provider.GetPod(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.NotNil(t, podLocal)

	podLocalList, err := provider.GetPods(ctx)
	assert.NoError(t, err)
	assert.Equal(t, len(podLocalList), 1)

	podStatus, err = provider.GetPodStatus(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.Equal(t, podStatus.Phase, corev1.PodPending)

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-container",
			BizState:   "ACTIVATED",
			BizVersion: "0.0.1",
		},
	})

	podStatus, err = provider.GetPodStatus(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.Equal(t, podStatus.Phase, corev1.PodRunning)

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-container",
			BizState:   "DEACTIVATED",
			BizVersion: "0.0.1",
		},
	})

	podStatus, err = provider.GetPodStatus(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.Equal(t, podStatus.Phase, corev1.PodRunning)

	podCopy := pod.DeepCopy()
	podCopy.Spec.Containers = []corev1.Container{
		{
			Name:  "test-container2",
			Image: "test-image2",
		},
	}
	err = provider.UpdatePod(ctx, podCopy)
	assert.NoError(t, err)
	podLocal, err = provider.GetPod(ctx, pod.Namespace, pod.Name)
	assert.NoError(t, err)
	assert.NotNil(t, podLocal)
	assert.Equal(t, podCopy.Spec.Containers[0].Name, podLocal.Spec.Containers[0].Name)

	assert.NoError(t, provider.DeletePod(ctx, podCopy))
	provider.SyncBizInfo([]ark.ArkBizInfo{})
	assert.Eventually(t, func() bool {
		podLocal, err = provider.GetPod(ctx, pod.Namespace, pod.Name)
		assert.NoError(t, err)
		return podLocal == nil
	}, time.Second*5, time.Millisecond*100)

	podCopy = pod.DeepCopy()
	podCopy.Spec.Containers = []corev1.Container{
		{
			Name:  "test-container",
			Image: "test-image",
			Env: []corev1.EnvVar{
				{
					Name:  "BIZ_VERSION",
					Value: "0.0.1",
				},
			},
		},
	}
	provider.runtimeInfoStore.PutPod(pod)

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-container",
			BizState:   "ACTIVATED",
			BizVersion: "0.0.1",
		},
		{
			BizName:    "test-container2",
			BizState:   "ACTIVATED",
			BizVersion: "0.0.1",
		},
		{
			BizName:    "test-container3",
			BizState:   "RESOLVED",
			BizVersion: "0.0.1",
		},
		{
			BizName:    "test-container4",
			BizState:   "DEACTIVATED",
			BizVersion: "0.0.1",
		},
	})
}

func TestBaseProvider_BizInstallCheck(t *testing.T) {
	ctx := context.Background()

	provider := NewBaseProvider("default", "127.0.0.1", "test_node", fake.NewSimpleClientset(), nil)
	identity := "test-biz:0.0.1"
	err := provider.handleInstallOperation(ctx, identity)
	assert.NoError(t, err)

	provider.runtimeInfoStore.PutPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test-biz",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "0.0.1",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{},
	})

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-biz",
			BizState:   "ACTIVATED",
			BizVersion: "0.0.1",
		},
	})

	err = provider.handleInstallOperation(ctx, identity)
	assert.NoError(t, err)

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-biz",
			BizState:   "RESOLVED",
			BizVersion: "0.0.1",
		},
	})

	err = provider.handleInstallOperation(ctx, identity)
	assert.NoError(t, err)

	provider.SyncBizInfo([]ark.ArkBizInfo{
		{
			BizName:    "test-biz",
			BizState:   "DEACTIVATED",
			BizVersion: "0.0.1",
		},
	})

	err = provider.handleInstallOperation(ctx, identity)
	assert.Error(t, err)
}
