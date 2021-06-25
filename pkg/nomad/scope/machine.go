package scope

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	infrav1 "github.com/spectrocloud/cluster-api-provider-nomad/api/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/klogr"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	DNSDomain = "sc"
)

// MachineScopeParams defines the input parameters used to create a new Scope.
type MachineScopeParams struct {
	Client         client.Client
	Logger         logr.Logger
	Cluster        *clusterv1.Cluster
	ClusterScope   *ClusterScope
	Machine        *clusterv1.Machine
	NomadMachine   *infrav1.NomadMachine
	ControllerName string

	Tracker *remote.ClusterCacheTracker
}

// MachineScope defines the basic context for an actuator to operate upon.
type MachineScope struct {
	logr.Logger
	client      client.Client
	patchHelper *patch.Helper

	Cluster      *clusterv1.Cluster
	ClusterScope *ClusterScope

	Machine      *clusterv1.Machine
	NomadMachine *infrav1.NomadMachine

	controllerName string
	tracker        *remote.ClusterCacheTracker
}

// NewMachineScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewMachineScope(params MachineScopeParams) (*MachineScope, error) {
	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	//session, serviceLimiters, err := sessionForRegion(params.NomadMachine.Spec.Region, params.Endpoints)
	//if err != nil {
	//	return nil, errors.Errorf("failed to create nomad session: %v", err)
	//}

	helper, err := patch.NewHelper(params.NomadMachine, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}
	return &MachineScope{
		Logger:         params.Logger,
		Machine:        params.Machine,
		NomadMachine:   params.NomadMachine,
		Cluster:        params.Cluster,
		ClusterScope:   params.ClusterScope,
		patchHelper:    helper,
		client:         params.Client,
		tracker:        params.Tracker,
		controllerName: params.ControllerName,
	}, nil
}

// PatchObject persists the machine configuration and status.
func (m *MachineScope) PatchObject() error {

	applicableConditions := []clusterv1.ConditionType{
		infrav1.MachineDeployedCondition,
	}

	if m.IsControlPlane() {
		applicableConditions = append(applicableConditions, infrav1.DNSAttachedCondition)
	}
	// Always update the readyCondition by summarizing the state of other conditions.
	// A step counter is added to represent progress during the provisioning process (instead we are hiding it during the deletion process).
	conditions.SetSummary(m.NomadMachine,
		conditions.WithConditions(applicableConditions...),
		conditions.WithStepCounterIf(m.NomadMachine.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return m.patchHelper.Patch(
		context.TODO(),
		m.NomadMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.MachineDeployedCondition,
		}},
	)
}

// Close closes the current scope persisting the cluster configuration and status.
func (m *MachineScope) Close() error {
	return m.PatchObject()
}

// SetAddresses sets the AWSMachine address status.
func (m *MachineScope) SetAddresses(addrs []clusterv1.MachineAddress) {
	m.NomadMachine.Status.Addresses = addrs
}

// SetReady sets the NomadMachine Ready Status
func (m *MachineScope) SetReady() {
	m.NomadMachine.Status.Ready = true
}

// IsReady gets NomadMachine Ready Status
func (m *MachineScope) IsReady() bool {
	return m.NomadMachine.Status.Ready
}

// SetNotReady sets the NomadMachine Ready Status to false
func (m *MachineScope) SetNotReady() {
	m.NomadMachine.Status.Ready = false
}

// SetFailureMessage sets the NomadMachine status failure message.
func (m *MachineScope) SetFailureMessage(v error) {
	m.NomadMachine.Status.FailureMessage = pointer.StringPtr(v.Error())
}

// SetFailureReason sets the NomadMachine status failure reason.
func (m *MachineScope) SetFailureReason(v capierrors.MachineStatusError) {
	m.NomadMachine.Status.FailureReason = &v
}

// IsControlPlane returns true if the machine is a control plane.
func (m *MachineScope) IsControlPlane() bool {
	return util.IsControlPlaneMachine(m.Machine)
}

// Role returns the machine role from the labels.
func (m *MachineScope) Role() string {
	if util.IsControlPlaneMachine(m.Machine) {
		return "control-plane"
	}
	return "node"
}

// Id returns the NomadMachine instance id by parsing Spec.ProviderID.
func (m *MachineScope) Id() string {
	return m.Machine.Name
}

// GetDNSName returns the NomadMachine instance id by parsing Spec.ProviderID.
func (m *MachineScope) GetDNSName() string {
	return fmt.Sprintf("%s.%s", m.Machine.Name, DNSDomain)
}

// GetProviderID returns the NomadMachine providerID from the spec.
func (m *MachineScope) GetProviderID() string {
	if m.NomadMachine.Spec.ProviderID != nil {
		return *m.NomadMachine.Spec.ProviderID
	}
	return ""
}

// SetProviderID sets the NomadMachine providerID in spec.
func (m *MachineScope) SetProviderID(jobID, deploymentID string) {
	providerID := fmt.Sprintf("nomad:///%s/%s", jobID, deploymentID)
	m.NomadMachine.Spec.ProviderID = pointer.StringPtr(providerID)
}

// SetFailureDomain sets the NomadMachine systemID in spec.
//func (m *MachineScope) SetFailureDomain(availabilityZone string) {
//	m.NomadMachine.Spec.FailureDomain = pointer.StringPtr(availabilityZone)
//}

// SetEvalID sets the NomadMachine systemID in spec.
//func (m *MachineScope) SetEvalID(evalID string) {
//	m.NomadMachine.Status.EvalID = pointer.StringPtr(evalID)
//}

// GetDeploymentStatus returns the NomadMachine instance state from the status.
func (m *MachineScope) GetDeploymentStatus() *infrav1.MachineStatus {
	return m.NomadMachine.Status.DeploymentStatus
}

// SetDeploymentStatus sets the NomadMachine status instance state.
func (m *MachineScope) SetDeploymentStatus(v infrav1.MachineStatus) {
	m.NomadMachine.Status.DeploymentStatus = &v
}

// SetDeploymentStatusDescription sets the NomadMachine status description
func (m *MachineScope) SetDeploymentStatusDescription(s string) {
	m.NomadMachine.Status.StatusDescription = &s
}

//func (m *MachineScope) SetPowered(powered bool) {
//	m.NomadMachine.Status.MachinePowered = powered
//}

// GetMachineHostname retrns the hostname
//func (m *MachineScope) GetMachineHostname() string {
//	if m.NomadMachine.Status.Hostname != nil {
//		return *m.NomadMachine.Status.Hostname
//	}
//	return ""
//}

// SetMachineHostname sets the hostname
//func (m *MachineScope) SetMachineHostname(hostname string) {
//	m.NomadMachine.Status.Hostname = &hostname
//}

func (m *MachineScope) MachineIsRunning() bool {
	state := m.GetDeploymentStatus()
	return state != nil && infrav1.MachineRunningStates.Has(string(*state))
}

func (m *MachineScope) MachineIsOperational() bool {
	state := m.GetDeploymentStatus()
	return state != nil && infrav1.MachineOperationalStates.Has(string(*state))
}

func (m *MachineScope) MachineIsInKnownState() bool {
	state := m.GetDeploymentStatus()
	return state != nil && infrav1.MachineKnownStates.Has(string(*state))
}

// GetRawBootstrapData returns the bootstrap data from the secret in the Machine's bootstrap.dataSecretName.
func (m *MachineScope) GetRawBootstrapData() ([]byte, error) {
	if m.Machine.Spec.Bootstrap.DataSecretName == nil {
		return nil, errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	}

	namespace := m.Machine.Namespace

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: *m.Machine.Spec.Bootstrap.DataSecretName}
	if err := m.client.Get(context.TODO(), key, secret); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve bootstrap data secret for NomadMachine %s/%s", namespace, m.Machine.Name)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return nil, errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	return value, nil
}

// SetNodeProviderID patches the node with the ID
func (m *MachineScope) SetNodeProviderID() error {
	ctx := context.TODO()
	remoteClient, err := m.tracker.GetClient(ctx, util.ObjectKey(m.Cluster))
	if err != nil {
		return err
	}

	node := &corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: m.Machine.Name}, node); err != nil {
		return err
	}

	providerID := m.GetProviderID()
	if node.Spec.ProviderID == providerID {
		return nil
	}

	patchHelper, err := patch.NewHelper(node, remoteClient)
	if err != nil {
		return err
	}

	node.Spec.ProviderID = providerID

	return patchHelper.Patch(ctx, node)
}
