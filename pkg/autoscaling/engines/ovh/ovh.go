package ovh

import (
	"github.com/containership/cluster-manager/pkg/log"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
	rest "k8s.io/client-go/rest"

	nodepool "github.com/containership/cerebral/pkg/apis/nodepools.kube.cloud.ovh.com/v1alpha1"
	"github.com/containership/cerebral/pkg/autoscaling"
	nodepoolClient "github.com/containership/cerebral/pkg/autoscaling/engines/ovh/clientset/versioned"
)

const (
	nodePoolIDLabelKey = "nodepool"
)

type Engine struct {
	name       string
	clientset  nodepoolClient.Interface
	nodeLister corelistersv1.NodeLister
}

func NewClient(name string, config *rest.Config, nodeLister corelistersv1.NodeLister) (autoscaling.Engine, error) {
	if name == "" {
		return nil, errors.New("name must be provided")
	}

	if nodeLister == nil {
		return nil, errors.New("node lister must be provided")
	}

	clientset, err := nodepoolClient.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a nodepool client")
	}

	return Engine{
		name:       name,
		clientset:  clientset,
		nodeLister: nodeLister,
	}, nil
}

// Name returns the name of the engine
func (e Engine) Name() string {
	return e.name
}

// SetTargetNodeCount takes action to scale the target node pool
func (e Engine) SetTargetNodeCount(nodeSelectors map[string]string, numNodes int, strategy string) (bool, error) {
	if numNodes < 0 {
		return false, errors.New("cannot scale below 0")
	}

	log.Infof("OVH AutoscalingEngine %s is requesting OVH to scale to %d", e.Name(), numNodes)

	switch strategy {
	// random is the default for this engine
	case "random", "":

		scaled, err := e.scaleLabelSpecifiedNodePool(nodeSelectors, numNodes)
		if err != nil {
			return false, errors.Wrap(err, "unable to scale OVH cluster")
		}

		return scaled, nil

	default:
		return false, errors.Errorf("unknown scale strategy %s", strategy)
	}
}

func (e Engine) scaleLabelSpecifiedNodePool(nodeSelectors map[string]string, numNodes int) (bool, error) {
	id, err := getRandomNodePoolIDToScale(nodeSelectors, nodePoolIDLabelKey, numNodes, e.nodeLister)
	if err != nil {
		return false, errors.Wrap(err, "OVH engine getting node pool ID to scale")
	}

	if id == "" {
		return false, nil
	}

	np, err := e.clientset.OVHV1alpha1().NodePools().Get(id, v1.GetOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to get the nodepool")
	}

	err = e.scaleNodePoolToCount(np, numNodes)
	if err != nil {
		return false, errors.Wrapf(err, "scaling node pool with node selectors %s", nodeSelectors)
	}

	return true, nil
}

func (e Engine) scaleNodePoolToCount(nodePool *nodepool.NodePool, numNodes int) error {
	nodePool.Spec.Desired = numNodes
	if _, err := e.clientset.OVHV1alpha1().NodePools().Update(nodePool); err != nil {
		return errors.Wrap(err, "failed to scale node pool")
	}
	return nil
}
