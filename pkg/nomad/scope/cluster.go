package scope

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	infrav1 "github.com/spectrocloud/cluster-api-provider-nomad/api/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sync"
	"time"
)

var (
	ConsulDomain = "service.consul"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	Client              client.Client
	Logger              logr.Logger
	Cluster             *clusterv1.Cluster
	NomadCluster        *infrav1.NomadCluster
	ControllerName      string
	Tracker             *remote.ClusterCacheTracker
	ClusterEventChannel chan event.GenericEvent
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	logr.Logger
	client      client.Client
	patchHelper *patch.Helper

	Cluster             *clusterv1.Cluster
	NomadCluster        *infrav1.NomadCluster
	controllerName      string
	tracker             *remote.ClusterCacheTracker
	clusterEventChannel chan event.GenericEvent
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	//session, serviceLimiters, err := sessionForRegion(params.NomadCluster.Spec.Region, params.Endpoints)
	//if err != nil {
	//	return nil, errors.Errorf("failed to create nomad session: %v", err)
	//}

	helper, err := patch.NewHelper(params.NomadCluster, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}
	return &ClusterScope{
		Logger:              params.Logger,
		client:              params.Client,
		Cluster:             params.Cluster,
		NomadCluster:        params.NomadCluster,
		patchHelper:         helper,
		tracker:             params.Tracker,
		controllerName:      params.ControllerName,
		clusterEventChannel: params.ClusterEventChannel,
	}, nil
}

// PatchObject persists the cluster configuration and status.
func (s *ClusterScope) PatchObject() error {
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	conditions.SetSummary(s.NomadCluster,
		conditions.WithConditions(
			infrav1.DNSReadyCondition,
			infrav1.APIServerAvailableCondition,
		),
		conditions.WithStepCounterIf(s.NomadCluster.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return s.patchHelper.Patch(
		context.TODO(),
		s.NomadCluster,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.DNSReadyCondition,
			infrav1.APIServerAvailableCondition,
		}},
	)
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *ClusterScope) Close() error {
	return s.PatchObject()
}

// APIServerPort returns the APIServerPort to use when creating the load balancer.
func (s *ClusterScope) APIServerPort() int {
	if s.Cluster.Spec.ClusterNetwork != nil && s.Cluster.Spec.ClusterNetwork.APIServerPort != nil {
		return int(*s.Cluster.Spec.ClusterNetwork.APIServerPort)
	}
	return 6443
}

// SetDNSName sets the Network systemID in spec.
func (s *ClusterScope) SetDNSName(dnsName string) {
	s.NomadCluster.Status.Network.DNSName = dnsName
}

// GetDNSName sets the Network systemID in spec.
// This can't do a lookup on Status.Network.ConsulDomain name since it's derived from here
func (s *ClusterScope) GetDNSName() string {
	return fmt.Sprintf("%s.%s", s.Cluster.Name, ConsulDomain)
}

//// GetClusterNomadMachines all Nomad machines NOT being deleted
//func (s *ClusterScope) GetClusterNomadMachines() ([]*infrav1.NomadMachine, error) {
//
//	machineList := &infrav1.NomadMachineList{}
//	labels := map[string]string{clusterv1.ClusterLabelName: s.Cluster.Name}
//
//	if err := s.client.List(
//		context.TODO(),
//		machineList,
//		client.InNamespace(s.Cluster.Namespace),
//		client.MatchingLabels(labels)); err != nil {
//		return nil, errors.Wrap(err, "failed to list machines")
//	}
//
//	var machines []*infrav1.NomadMachine
//	for i := range machineList.Items {
//		m := &machineList.Items[i]
//		machines = append(machines, m)
//		// TODO need active?
//		//if m.DeletionTimestamp.IsZero() {
//		//}
//	}
//
//	return machines, nil
//}

var (
	// apiServerTriggers is used to prevent multiple goroutines for a single
	// Cluster that poll to see if the target API server is online.
	apiServerTriggers   = map[types.UID]struct{}{}
	apiServerTriggersMu sync.Mutex
)

func (s *ClusterScope) ReconcileNomadClusterWhenAPIServerIsOnline() {
	if s.Cluster.Status.ControlPlaneInitialized {
		s.Info("skipping reconcile when API server is online",
			"reason", "controlPlaneInitialized")
		return
	} else if !s.Cluster.DeletionTimestamp.IsZero() {
		s.Info("skipping reconcile when API server is online",
			"reason", "controlPlaneDeleting")
		return
	}

	apiServerTriggersMu.Lock()
	defer apiServerTriggersMu.Unlock()
	if _, ok := apiServerTriggers[s.Cluster.UID]; ok {
		s.Info("skipping reconcile when API server is online",
			"reason", "alreadyPolling")
		return
	}
	apiServerTriggers[s.Cluster.UID] = struct{}{}
	go func() {
		// Block until the target API server is online.

		s.Info("start polling API server for online check")
		_ = wait.PollImmediateInfinite(time.Second*1, func() (bool, error) { return s.IsAPIServerOnline() })
		s.Info("stop polling API server for online check")
		s.Info("triggering GenericEvent", "reason", "api-server-online")
		s.clusterEventChannel <- event.GenericEvent{
			Meta:   s.NomadCluster,
			Object: s.NomadCluster,
		}

		apiServerTriggersMu.Lock()
		delete(apiServerTriggers, s.Cluster.UID)
		apiServerTriggersMu.Unlock()

		//// Once the control plane has been marked as initialized it is safe to
		//// remove the key from the map that prevents multiple goroutines from
		//// polling the API server to see if it is online.
		//s.Info("start polling for control plane initialized")
		//wait.PollImmediateInfinite(time.Second*1, func() (bool, error) { return r.isControlPlaneInitialized(ctx), nil }) // nolint:errcheck
		//s.Info("stop polling for control plane initialized")
	}()
}

func (s *ClusterScope) IsAPIServerOnline() (bool, error) {

	ctx := context.TODO()

	cluster := &clusterv1.Cluster{}
	if err := s.client.Get(ctx, util.ObjectKey(s.Cluster), cluster); err != nil {
		return false, err
	} else if !cluster.DeletionTimestamp.IsZero() {
		s.Info("Cluster is deleting; abort APIServerOnline check", "cluster", cluster.Name)
		return false, errors.New("Cluster is deleting; abort IsAPIServerOnline")
	}

	remoteClient, err := s.tracker.GetClient(ctx, util.ObjectKey(s.Cluster))
	if err != nil {
		s.V(2).Info("Waiting for online server to come online")
		return false, nil
	}

	err = remoteClient.List(ctx, new(v1.NodeList))

	return err == nil, nil
}

//func (s *ClusterScope) isControlPlaneInitialized(ctx context.Context, s *scope.ClusterScope) bool {
//	cluster := &clusterv1.Cluster{}
//	clusterKey := client.ObjectKey{Namespace: s.Cluster.Namespace, Name: s.Cluster.Name}
//	if err := s.Client.Get(ctx, clusterKey, cluster); err != nil {
//		if !apierrors.IsNotFound(err) {
//			s.Error(err, "failed to get updated cluster object while checking if control plane is initialized")
//			return false
//		}
//		s.Info("exiting early because cluster no longer exists")
//		return true
//	}
//	return cluster.Status.ControlPlaneInitialized
//}
