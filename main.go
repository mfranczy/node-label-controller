package main

import (
	"flag"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

func main() {
	var (
		master     string
		kubeConfig string
	)
	klog.InitFlags(nil)

	flag.StringVar(&master, "master", "", "k8s master address")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "path to kubeconfig file")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags(master, kubeConfig)
	if err != nil {
		klog.Fatal(err)
	}

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}

	nodeListWatcher := cache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "nodes", "", fields.Everything())
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(nodeListWatcher, &v1.Node{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				klog.Error(err)
				return
			}
			queue.Add(key)
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err != nil {
				klog.Error(err)
				return
			}
			queue.Add(key)
		},
	}, cache.Indexers{})

	controller := NewController(indexer, informer, queue, clientSet)

	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(stop)

	select {}
}
