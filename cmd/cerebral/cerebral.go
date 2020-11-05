package main

import (
	"flag"
	"os"
	"runtime"
	"time"

	"github.com/pkg/errors"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/containership/cerebral/pkg/buildinfo"
	cerebral "github.com/containership/cerebral/pkg/client/clientset/versioned"
	cerebralscheme "github.com/containership/cerebral/pkg/client/clientset/versioned/scheme"
	cinformers "github.com/containership/cerebral/pkg/client/informers/externalversions"
	"github.com/containership/cerebral/pkg/controller"

	"github.com/containership/cluster-manager/pkg/log"
)

func main() {
	log.Info("Starting Cerebral...")
	log.Infof("Version: %s", buildinfo.String())
	log.Infof("Go Version: %s", runtime.Version())

	// We don't have any of our own flags to parse, but k8s packages want to
	// use klog and we have to pass flags to that to configure it to behave
	// in a sane way.
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	config, err := determineConfig()
	if err != nil {
		log.Fatal(err)
	}

	kubeclientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %+v", err)
	}

	cerebralclientset, err := cerebral.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Cerebral clientset: %+v", err)
	}

	// Add cerebral scheme so we can record events
	cerebralscheme.AddToScheme(scheme.Scheme)

	kubeInformerFactory := informers.NewSharedInformerFactory(kubeclientset, 30*time.Second)
	cerebralInformerFactory := cinformers.NewSharedInformerFactory(cerebralclientset, 30*time.Second)

	stopCh := make(chan struct{})
	scaleMgr := controller.NewScaleManager(
		kubeclientset, kubeInformerFactory, cerebralclientset, cerebralInformerFactory)

	autoscalingGroupController := controller.NewAutoscalingGroupController(
		kubeclientset, kubeInformerFactory, cerebralclientset, cerebralInformerFactory,
		scaleMgr.ScaleRequestChan())

	metricsController := controller.NewMetrics(
		kubeclientset, kubeInformerFactory, cerebralclientset, cerebralInformerFactory,
		scaleMgr.ScaleRequestChan())

	metricsBackendController := controller.NewMetricsBackend(
		kubeclientset, kubeInformerFactory, cerebralclientset, cerebralInformerFactory)

	autoscalingEngineController := controller.NewAutoscalingEngine(
		kubeclientset, kubeInformerFactory, cerebralclientset, cerebralInformerFactory, config)

	kubeInformerFactory.Start(stopCh)
	cerebralInformerFactory.Start(stopCh)

	go func() {
		if err := scaleMgr.Run(stopCh); err != nil {
			log.Fatalf("Error running scale manager: %s", err.Error())
		}
	}()

	go func() {
		if err := autoscalingGroupController.Run(1, stopCh); err != nil {
			log.Fatalf("Error running AutoscalingGroupController: %s", err.Error())
		}
	}()

	go func() {
		if err := metricsBackendController.Run(1, stopCh); err != nil {
			log.Fatalf("Error running MetricsBackendController: %s", err.Error())
		}
	}()

	go func() {
		if err := autoscalingEngineController.Run(1, stopCh); err != nil {
			log.Fatalf("Error running AutoscalingEngineController: %s", err.Error())
		}
	}()

	go func() {
		if err := metricsController.Run(1, stopCh); err != nil {
			log.Fatalf("Error running MetricsController: %s", err.Error())
		}
	}()

	<-stopCh
	log.Fatal("There was an error while running the scale manager and controllers")
}

// determineConfig determines if we are running in a cluster or outside
// and gets the appropriate configuration to talk with Kubernetes.
func determineConfig() (*rest.Config, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	var config *rest.Config
	var err error

	// determine whether to use in cluster config or out of cluster config
	// if kubeconfigPath is not specified, default to in cluster config
	// otherwise, use out of cluster config
	if kubeconfigPath == "" {
		log.Info("Using in cluster k8s config")
		config, err = rest.InClusterConfig()
	} else {
		log.Info("Using out of cluster k8s config: ", kubeconfigPath)

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	if err != nil {
		return nil, errors.Wrap(err, "determine Kubernetes config failed")
	}

	return config, nil
}
