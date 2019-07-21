package main

import (
	"fmt"
	"regexp"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const kubermaticLabel = "kubermatic.io/uses-container-linux"

var containerLinuxReg = regexp.MustCompile(`Container Linux by CoreOS`)

type Controller struct {
	indexer   cache.Indexer
	informer  cache.Controller
	queue     workqueue.RateLimitingInterface
	clientSet *kubernetes.Clientset
}

func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	if c.queue.NumRequeues(key) < 3 {
		c.queue.AddRateLimited(key)
		klog.Infof("Error updating node label %v: %v", key, err)
		return
	}

	c.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("Dropping node %q out of the queue: %v", key, err)
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()
	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	go wait.Until(c.runWorker, time.Second, stopCh)
	<-stopCh
	klog.Info("Stopping node controller")
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	nodeName := key.(string)

	obj, exists, err := c.indexer.GetByKey(nodeName)
	if err != nil || !exists {
		if !exists {
			err = fmt.Errorf("Indexer does not contain node %s", nodeName)
		}
		c.handleErr(err, key)
		return true
	}

	node := obj.(*v1.Node)
	if containerLinuxReg.MatchString(node.Status.NodeInfo.OSImage) {
		if val, ok := node.Labels[kubermaticLabel]; !ok || ok && val != "true" {
			klog.Infof("Updating node %s", nodeName)
			data := []byte(fmt.Sprintf(`{"metadata": { "labels": {"%s": "true"}}}`, kubermaticLabel))
			_, err = c.clientSet.CoreV1().Nodes().Patch(nodeName, types.StrategicMergePatchType, data)
		}
	}

	c.handleErr(err, key)
	defer c.queue.Done(key)
	return true
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func NewController(indexer cache.Indexer, informer cache.Controller, queue workqueue.RateLimitingInterface, clientSet *kubernetes.Clientset) *Controller {
	return &Controller{indexer, informer, queue, clientSet}
}
