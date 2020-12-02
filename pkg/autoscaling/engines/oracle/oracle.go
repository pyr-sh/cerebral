package oracle

import (
	"context"

	"github.com/containership/cluster-manager/pkg/log"
	"github.com/oracle/oci-go-sdk/v30/common"
	"github.com/oracle/oci-go-sdk/v30/containerengine"
	"github.com/pkg/errors"
	corelistersv1 "k8s.io/client-go/listers/core/v1"

	"github.com/containership/cerebral/pkg/autoscaling"
)

type Engine struct {
	name       string
	clusterID  string
	ce         containerengine.ContainerEngineClient
	nodeLister corelistersv1.NodeLister
}

func NewClient(name string, configuration map[string]string, nodeLister corelistersv1.NodeLister) (autoscaling.Engine, error) {
	if name == "" {
		return nil, errors.New("name must be provided")
	}

	if nodeLister == nil {
		return nil, errors.New("node lister must be provided")
	}

	oracleConfig := common.DefaultConfigProvider()
	ce, err := containerengine.NewContainerEngineClientWithConfigurationProvider(oracleConfig)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a new container engine client")
	}

	clusterID, ok := configuration["cluster_id"]
	if !ok {
		return nil, errors.New("cluster id is required")
	}

	return Engine{
		name:       name,
		clusterID:  clusterID,
		ce:         ce,
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

	log.Infof("Oracle AutoscalingEngine %s is requesting Oracle to scale to %d", e.Name(), numNodes)

	switch strategy {
	// random is the default for this engine
	case "random", "":

		scaled, err := e.scaleLabelSpecifiedNodePool(nodeSelectors, numNodes)
		if err != nil {
			return false, errors.Wrap(err, "unable to scale Oracle cluster")
		}

		return scaled, nil

	default:
		return false, errors.Errorf("unknown scale strategy %s", strategy)
	}
}

func (e Engine) scaleLabelSpecifiedNodePool(nodeSelectors map[string]string, numNodes int) (bool, error) {
	name, compartment, err := getRandomPoolToScale(nodeSelectors, numNodes, e.nodeLister)
	if err != nil {
		return false, errors.Wrap(err, "Oracle engine getting node pool ID to scale")
	}
	if name == "" {
		return false, nil
	}

	listResp, err := e.ce.ListNodePools(context.TODO(), containerengine.ListNodePoolsRequest{
		CompartmentId: common.String(compartment),
		ClusterId:     common.String(e.clusterID),
		Name:          common.String(name),
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to list nodepools")
	}

	if len(listResp.Items) == 0 {
		return false, errors.New("did not find any node pools")
	}

	nodePool := listResp.Items[0]
	if _, err := e.ce.UpdateNodePool(context.TODO(), containerengine.UpdateNodePoolRequest{
		NodePoolId: nodePool.Id,
		UpdateNodePoolDetails: containerengine.UpdateNodePoolDetails{
			QuantityPerSubnet: common.Int(numNodes),
		},
	}); err != nil {
		return false, errors.Wrap(err, "failed to update the node pool")
	}

	log.Infof("scaled Oracle cluster %s nodepool %s (%s) to %d nodes", e.clusterID, *nodePool.Id, name, numNodes)

	return true, nil
}
