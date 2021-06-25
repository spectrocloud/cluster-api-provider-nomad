/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/dns"
	nomadmachine "github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/machine"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/scope"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	infrav1 "github.com/spectrocloud/cluster-api-provider-nomad/api/v1alpha3"
)

//var ErrRequeueDNS = errors.New("need to requeue DNS")

// NomadMachineReconciler reconciles a NomadMachine object
type NomadMachineReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Tracker  *remote.ClusterCacheTracker
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nomadmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nomadmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

func (r *NomadMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := r.Log.WithValues("nomadmachine", req.Name)

	ctx := context.TODO()

	// Fetch the NomadMachine instance.
	nomadMachine := &infrav1.NomadMachine{}
	err := r.Get(ctx, req.NamespacedName, nomadMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, nomadMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(cluster, nomadMachine) {
		log.Info("NomadMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// Get Infra cluster
	nomadCluster := &infrav1.NomadCluster{}
	infraClusterName := client.ObjectKey{
		Namespace: nomadMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}

	if err := r.Client.Get(ctx, infraClusterName, nomadCluster); err != nil {
		log.Info("NomadCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Create the cluster scope
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:         r.Client,
		Logger:         log,
		Cluster:        cluster,
		Tracker:        r.Tracker,
		NomadCluster:   nomadCluster,
		ControllerName: "nomadmachine",
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Logger:       log,
		Client:       r.Client,
		Tracker:      r.Tracker,
		Cluster:      cluster,
		ClusterScope: clusterScope,
		Machine:      machine,
		NomadMachine: nomadMachine,
	})
	if err != nil {
		log.Error(err, "failed to create scope")
		return ctrl.Result{}, err
	}

	// Always close the scope when exiting this function so we can persist any NomadMachine changes.
	defer func() {
		if err := machineScope.Close(); err != nil && rerr == nil {
			rerr = err
		}
	}()

	// Handle deleted machines
	if !nomadMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machineScope, clusterScope)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, machineScope, clusterScope)
}

func (r *NomadMachineReconciler) reconcileDelete(_ context.Context, machineScope *scope.MachineScope, _ *scope.ClusterScope) (ctrl.Result, error) {
	machineScope.Info("Reconciling NomadMachine delete")

	nomadMachine := machineScope.NomadMachine

	machineSvc := nomadmachine.NewService(machineScope)

	if err := r.reconcileDNSAttachment(machineScope); err != nil {
		machineScope.Error(err, "failed to reconcile DNS attachment")
		return ctrl.Result{}, err
	}

	// TODO
	if err := machineSvc.ReleaseMachine(machineScope.Id()); err != nil {
		machineScope.Error(err, "failed to release machine")
		return ctrl.Result{}, err
	}

	conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, clusterv1.DeletedReason, clusterv1.ConditionSeverityInfo, "")
	r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeNormal, "SuccessfulRelease", "Released instance %q", machineScope.Id())

	// Machine is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(nomadMachine, infrav1.MachineFinalizer)

	return reconcile.Result{}, nil
}

func (r *NomadMachineReconciler) reconcileNormal(_ context.Context, machineScope *scope.MachineScope, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	machineScope.Info("Reconciling NomadMachine")

	nomadMachine := machineScope.NomadMachine

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(nomadMachine, infrav1.MachineFinalizer) {
		controllerutil.AddFinalizer(nomadMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	if !machineScope.Cluster.Status.InfrastructureReady {
		machineScope.Info("Cluster infrastructure is not ready yet")
		conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Make sure bootstrap data is available and populated.
	if machineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		machineScope.Info("Bootstrap data secret reference is not yet available")
		conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	machineSvc := nomadmachine.NewService(machineScope)

	m, err := r.deployMachine(machineScope, machineSvc)
	if err != nil {
		machineScope.Error(err, "unable to create m")
		conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.MachineDeployFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	// Make sure Spec.ProviderID and Spec.InstanceID are always set.
	machineScope.SetProviderID(m.JobID, m.DeploymentID)
	//machineScope.SetFailureDomain(m.AvailabilityZone)

	existingMachineState := machineScope.GetDeploymentStatus()
	machineScope.Logger = machineScope.Logger.WithValues("state", m.Status, "m-id", machineScope.Id())
	machineScope.SetDeploymentStatus(m.Status)
	machineScope.SetDeploymentStatusDescription(m.StatusDescription)

	// Proceed to reconcile the NomadMachine state.
	if existingMachineState == nil || *existingMachineState != m.Status {
		machineScope.Info("Nomad m state changed", "old-state", existingMachineState)
	}

	switch s := m.Status; {
	//case s == infrav1.MachineStateReady, s == infrav1.MachineStateDiskErasing, s == infrav1.MachineStateReleasing, s == infrav1.MachineStateNew:
	//	machineScope.SetNotReady()
	//	machineScope.Info("Unexpected Nomad m termination")
	//	r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "MachineUnexpectedTermination", "Unexpected Nomad m termination")
	//	conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.MachineTerminatedReason, clusterv1.ConditionSeverityError, "")
	//	machineScope.SetFailureReason(capierrors.UpdateMachineError)
	//	machineScope.SetFailureMessage(errors.Errorf("Nomad machine state %q is unexpected", m.Status))
	//case machineScope.MachineIsInKnownState() && !m.Powered:
	//	machineScope.SetNotReady()
	//	machineScope.Info("Machine is powered off!")
	//	conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.MachinePoweredOffReason, clusterv1.ConditionSeverityWarning, "")
	case s == infrav1.MachineStateRunning:
		machineScope.SetNotReady()
		conditions.MarkFalse(machineScope.NomadMachine, infrav1.MachineDeployedCondition, infrav1.MachineDeployingReason, clusterv1.ConditionSeverityInfo, "")
	case s == infrav1.MachineStateSuccessful:
		machineScope.SetReady()
		conditions.MarkTrue(machineScope.NomadMachine, infrav1.MachineDeployedCondition)
	default:
		machineScope.SetNotReady()
		machineScope.Info("Nomad m state is undefined", "state", m.Status)
		r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "MachineUnhandledState", "Nomad m state is undefined")
		machineScope.SetFailureReason(capierrors.UpdateMachineError)
		machineScope.SetFailureMessage(errors.Errorf("Nomad m state %q is undefined", m.Status))
		conditions.MarkUnknown(machineScope.NomadMachine, infrav1.MachineDeployedCondition, "", "")
	}

	// tasks that can take place during all known instance states
	if machineScope.MachineIsInKnownState() {
		// TODO(saamalik) tags / labels

		// Set the address if good
		machineScope.SetAddresses(m.Addresses)

		if err := r.reconcileDNSAttachment(machineScope); err != nil {
			machineScope.Error(err, "failed to reconcile DNS attachment")
			return ctrl.Result{}, err
		}
	}

	// tasks that can only take place during operational instance states
	// Tried to not requeue here, but during error conditions (e.g: machine fails to boot)
	// there is no easy way to check other than a requeue
	if o, _ := clusterScope.IsAPIServerOnline(); !o {
		machineScope.Info("API Server is not online; requeue")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	} else if !machineScope.MachineIsOperational() {
		machineScope.Info("Machine is not operational; requeue")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	} else {
		if err := machineScope.SetNodeProviderID(); err != nil {
			machineScope.Error(err, "Unable to set Node hostname")
			r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "NodeProviderUpdateFailed", "Unable to set the node provider update")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *NomadMachineReconciler) deployMachine(machineScope *scope.MachineScope, machineSvc *nomadmachine.Service) (*infrav1.Machine, error) {
	machineScope.Info("Deploying on Nomad machine")

	userDataB64, userDataErr := r.resolveUserData(machineScope)
	if userDataErr != nil {
		return nil, errors.Wrapf(userDataErr, "failed to resolve userdata")
	}

	m, err := machineSvc.DeployMachine(userDataB64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to deploy NomadMachine instance")
	}

	return m, nil
}

func (r *NomadMachineReconciler) resolveUserData(machineScope *scope.MachineScope) (string, error) {
	userData, err := machineScope.GetRawBootstrapData()
	if err != nil {
		r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "FailedGetBootstrapData", err.Error())
		return "", err
	}

	// Base64 encode the userdata
	return string(userData), nil
}

func (r *NomadMachineReconciler) reconcileDNSAttachment(machineScope *scope.MachineScope) error {
	if !machineScope.IsControlPlane() {
		return nil
	}

	dnssvc := dns.NewService(machineScope)

	// If node being deleted - remove the service
	if !machineScope.NomadMachine.DeletionTimestamp.IsZero() || !machineScope.MachineIsRunning() {
		if err := dnssvc.DeregisterService(); err != nil {
			r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "FailedDetachControlPlaneDNS",
				"Failed to deregister control plane instance %q from DNS: failed to determine registration status: %v", machineScope.Id(), err)
			return errors.Wrapf(err, "machine %q - error detaching service", machineScope.Id())
		}

		conditions.MarkFalse(machineScope.NomadMachine, infrav1.DNSAttachedCondition, infrav1.DNSAttachPending, clusterv1.ConditionSeverityWarning, "")
		machineScope.NomadMachine.Status.DNSAttached = false
		return nil
	}

	// else add it
	if err := dnssvc.RegisterService(); err != nil {
		r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeWarning, "FailedAttachControlPlaneDNS",
			"Failed to attach control plane instance %q from DNS: failed to determine registration status: %v", machineScope.Id(), err)
		return errors.Wrapf(err, "machine %q - error registering service", machineScope.Id())
	}

	machineScope.NomadMachine.Status.DNSAttached = true
	conditions.MarkTrue(machineScope.NomadMachine, infrav1.DNSAttachedCondition)

	return nil
}

// SetupWithManager will add watches for this controller
func (r *NomadMachineReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToNomadMachines, err := util.ClusterToObjectsMapper(mgr.GetClient(), &infrav1.NomadMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.NomadMachine{}).
		WithOptions(options).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("NomadMachine")),
			},
			// v1alpha4
			//handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("NomadMachine"))),
		).
		Watches(
			&source.Kind{Type: &infrav1.NomadCluster{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.NomadClusterToNomadMachines)},
			// v1alpha4
			//handler.EnqueueRequestsFromMapFunc(r.NomadClusterToNomadMachines),
		).
		WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Build(r)
	if err != nil {
		return err
	}
	return c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: clusterToNomadMachines},
		predicates.ClusterUnpausedAndInfrastructureReady(r.Log),
	)
}

// v1alpha4
//func (r *NomadMachineReconciler) NomadClusterToNomadMachines(o client.Object) []ctrl.Request {

// NomadClusterToNomadMachines is a handler.ToRequestsFunc to be used to enqeue
// requests for reconciliation of NomadMachines.
func (r *NomadMachineReconciler) NomadClusterToNomadMachines(o handler.MapObject) []ctrl.Request {
	var result []ctrl.Request
	c, ok := o.Object.(*infrav1.NomadCluster)
	if !ok {
		panic(fmt.Sprintf("Expected a NomadCluster but got a %T", o))
	}

	cluster, err := util.GetOwnerCluster(context.TODO(), r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Spec.InfrastructureRef.Namespace, Name: m.Spec.InfrastructureRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}
