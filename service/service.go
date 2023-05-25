package service

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"smart-agent/config"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type K8SClient struct {
	cli *kubernetes.Clientset
}

type PortInfo struct {
	Name       string
	Protocol   string
	Port       int32
	NodePort   int32
	TargetPort string
}

type Service struct {
	SvcName   string
	ClusterIp string
	Ports     []PortInfo
}

func NewK8SClient(kubeconfig string) *K8SClient {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig == "" {
		home := homedir.HomeDir()
		loadingRules.ExplicitPath = filepath.Join(home, ".kube", "config")
	} else {
		loadingRules.ExplicitPath = kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		log.Printf("Failed to create client config: %v\n", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v\n", err)
		os.Exit(1)
	}
	return &K8SClient{
		cli: clientset,
	}
}

func (k8s *K8SClient) GetNamespaceServices(namespace string) []Service {
	var ret []Service
	services, err := k8s.cli.CoreV1().Services(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list services: %v\n", err)
		os.Exit(1)
	}
	for _, svc := range services.Items {
		cur := Service{
			SvcName:   svc.ObjectMeta.Name,
			ClusterIp: svc.Spec.ClusterIP,
			Ports:     []PortInfo{},
		}
		for _, port := range svc.Spec.Ports {
			p := PortInfo{
				Name:       port.Name,
				Protocol:   string(port.Protocol),
				Port:       port.Port,
				NodePort:   port.NodePort,
				TargetPort: port.TargetPort.String(),
			}
			cur.Ports = append(cur.Ports, p)
		}
		ret = append(ret, cur)
	}
	return ret
}

func (k8s *K8SClient) createConfigMap(data map[string]string) error {
	// Create a new ConfigMap object with the desired key-value pairs.
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.EtcdClientMapName,
			Namespace: config.Namespace,
		},
		Data: data,
	}

	// Create the ConfigMap in the specified namespace.
	_, err := k8s.cli.CoreV1().ConfigMaps(config.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (k8s *K8SClient) EtcdPut(key, value string) error {
	cm, err := k8s.cli.CoreV1().ConfigMaps(config.Namespace).Get(context.TODO(), config.EtcdClientMapName, metav1.GetOptions{})
	if err != nil {
		errmsg := err.Error()
		if strings.HasPrefix(errmsg, "configmaps") && strings.HasSuffix(errmsg, "not found") {
			return k8s.createConfigMap(map[string]string{key: value})
		}
		return err
	}
	cm.Data[key] = value
	_, err = k8s.cli.CoreV1().ConfigMaps(config.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}

func (k8s *K8SClient) EtcdGet(key string) (string, error) {
	cm, err := k8s.cli.CoreV1().ConfigMaps(config.Namespace).Get(context.TODO(), config.EtcdClientMapName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	data := cm.Data[key]
	return data, nil
}

func (k8s *K8SClient) EtcdDelete(key string) error {
	cm, err := k8s.cli.CoreV1().ConfigMaps(config.Namespace).Get(context.TODO(), config.EtcdClientMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	delete(cm.Data, key)
	_, err = k8s.cli.CoreV1().ConfigMaps(config.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}
