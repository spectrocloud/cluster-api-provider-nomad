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
	"github.com/pkg/errors"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/scope"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/spectrocloud/cluster-api-provider-nomad/api/v1alpha3"
)

// NomadClusterReconciler reconciles a NomadCluster object
type NomadClusterReconciler struct {
	client.Client
	Log                 logr.Logger
	Recorder            record.EventRecorder
	Tracker             *remote.ClusterCacheTracker
	GenericEventChannel chan event.GenericEvent
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nomadclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=nomadclusters/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a NomadCluster object and makes changes based on the state read
// and what is in the NomadCluster.Spec
func (r *NomadClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := r.Log.WithValues("nomadcluster", req.Name)

	ctx := context.TODO()

	// Fetch the NomadCluster instance
	nomadCluster := &infrav1.NomadCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, nomadCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, nomadCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on NomadCluster")
		return ctrl.Result{}, nil
	}

	// Create the scope.
	clusterScope, err := scope.NewClusterScope(scope.ClusterScopeParams{
		Client:              r.Client,
		Logger:              log,
		Cluster:             cluster,
		NomadCluster:        nomadCluster,
		Tracker:             r.Tracker,
		ClusterEventChannel: r.GenericEventChannel,
		ControllerName:      "nomadcluster",
	})
	if err != nil {
		return reconcile.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}

	// Always close the scope when exiting this function so we can persist any AWSCluster changes.
	defer func() {
		if err := clusterScope.Close(); err != nil && rerr == nil {
			rerr = err
		}
	}()

	// Support FailureDomains
	// In cloud providers this would likely look up which failure domains are supported and set the status appropriately.
	// so kCP will distribute the CPs across multiple failure domains
	failureDomains := make(clusterv1.FailureDomains)
	for _, az := range nomadCluster.Spec.FailureDomains {
		failureDomains[az] = clusterv1.FailureDomainSpec{
			ControlPlane: true,
		}
	}
	nomadCluster.Status.FailureDomains = failureDomains

	// Handle deleted clusters
	if !nomadCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, clusterScope)
}

func (r *NomadClusterReconciler) reconcileDelete(_ context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	clusterScope.Info("Reconciling NomadCluster delete")

	nomadCluster := clusterScope.NomadCluster

	// Wwait for the machines
	// https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/controllers/vspherecluster_controller.go#L221

	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(nomadCluster, infrav1.ClusterFinalizer)

	// TODO(saamalik) implement the recorder stuff (look at aws)

	return reconcile.Result{}, nil
}

//func (r *NomadClusterReconciler) reconcileDNSAttachments(clusterScope *scope.ClusterScope, dnssvc *dns.Service) error {
//	machines, err := clusterScope.GetClusterNomadMachines()
//	if err != nil {
//		return errors.Wrapf(err, "Unable to list all nomad machines")
//	}
//
//	var runningIpAddresses []string
//
//	currentIPs, err := dnssvc.GetAPIServerDNSRecords()
//	if err != nil {
//		return errors.Wrap(err, "Unable to get the dns resources")
//	}
//
//	machinesPendingAttachment := make([]*infrav1.NomadMachine, 0)
//	machinesPendingDetachment := make([]*infrav1.NomadMachine, 0)
//
//	for _, m := range machines {
//		if !IsControlPlaneMachine(m) {
//			continue
//		}
//
//		machineIP := getExternalMachineIP(m)
//		attached := currentIPs.Has(machineIP)
//		isRunningHealthy := IsRunning(m)
//
//		if !m.DeletionTimestamp.IsZero() || !isRunningHealthy {
//			if attached {
//				clusterScope.Info("Cleaning up IP on unhealthy machine", "machine", m.Name)
//				machinesPendingDetachment = append(machinesPendingDetachment, m)
//			}
//		} else if IsRunning(m) {
//			if !attached {
//				clusterScope.Info("Healthy machine without DNS attachment; attaching.", "machine", m.Name)
//				machinesPendingAttachment = append(machinesPendingAttachment, m)
//			}
//
//			runningIpAddresses = append(runningIpAddresses, machineIP)
//		}
//		//r.Recorder.Eventf(machineScope.NomadMachine, corev1.EventTypeNormal, "SuccessfulDetachControlPlaneDNS",
//		//	"Control plane instance %q is de-registered from load balancer", i.ID)
//		//runningIpAddresses = append(runningIpAddresses, m.)
//	}
//
//	if err := dnssvc.UpdateDNSAttachments(runningIpAddresses); err != nil {
//		return err
//	} else if len(machinesPendingAttachment) > 0 || len(machinesPendingDetachment) > 0 {
//		clusterScope.Info("Pending DNS attachments or detachments; will retry again")
//		return ErrRequeueDNS
//	}
//
//	return nil
//}

// IsControlPlaneMachine checks machine is a control plane node.
func IsControlPlaneMachine(m *infrav1.NomadMachine) bool {
	_, ok := m.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName]
	return ok
}

//// IsRunning returns if the machine is running
//func IsRunning(m *infrav1.NomadMachine) bool {
//	if !m.Status.MachinePowered {
//		return false
//	}
//
//	state := m.Status.MachineStatus
//	return state != nil && infrav1.MachineRunningStates.Has(string(*state))
//}
//
//func getExternalMachineIP(machine *infrav1.NomadMachine) string {
//	for _, i := range machine.Status.Addresses {
//		if i.Type == clusterv1.MachineExternalIP {
//			return i.Address
//		}
//	}
//	return ""
//}

func (r *NomadClusterReconciler) reconcileNormal(_ context.Context, clusterScope *scope.ClusterScope) (ctrl.Result, error) {
	clusterScope.Info("Reconciling NomadCluster")

	nomadCluster := clusterScope.NomadCluster

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(nomadCluster, infrav1.ClusterFinalizer) {
		controllerutil.AddFinalizer(nomadCluster, infrav1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	dnsName := clusterScope.GetDNSName()
	clusterScope.SetDNSName(dnsName)

	if nomadCluster.Status.Network.DNSName == "" {
		conditions.MarkFalse(nomadCluster, infrav1.DNSReadyCondition, infrav1.WaitForDNSNameReason, clusterv1.ConditionSeverityInfo, "")
		clusterScope.Info("Waiting on API server DNS name")
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	nomadCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: nomadCluster.Status.Network.DNSName,
		Port: clusterScope.APIServerPort(),
	}

	nomadCluster.Status.Ready = true

	// Mark the nomadCluster ready
	conditions.MarkTrue(nomadCluster, infrav1.DNSReadyCondition)

	// TODO do
	//if err := r.reconcileDNSAttachments(clusterScope, dnsService); err != nil {
	//	if errors.Is(err, ErrRequeueDNS) {
	//		return ctrl.Result{}, nil
	//		//return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	//	}
	//
	//	clusterScope.Error(err, "failed to reconcile load balancer")
	//	return reconcile.Result{}, err
	//
	//}

	clusterScope.ReconcileNomadClusterWhenAPIServerIsOnline()
	if k, _ := clusterScope.IsAPIServerOnline(); !k {
		conditions.MarkFalse(nomadCluster, infrav1.APIServerAvailableCondition, infrav1.APIServerNotReadyReason, clusterv1.ConditionSeverityWarning, "")
		return ctrl.Result{}, nil
	}

	conditions.MarkTrue(nomadCluster, infrav1.APIServerAvailableCondition)
	clusterScope.Info("API Server is available")

	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller
func (r *NomadClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.GenericEventChannel == nil {
		r.GenericEventChannel = make(chan event.GenericEvent)
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.NomadCluster{}).
		Watches(
			&source.Kind{Type: &infrav1.NomadMachine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.controlPlaneMachineToCluster),
			},
			// v1alpha4
			//handler.EnqueueRequestsFromMapFunc(r.controlPlaneMachineToCluster),
		).
		Watches(
			&source.Channel{Source: r.GenericEventChannel},
			&handler.EnqueueRequestForObject{},
		).
		WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Build(r)
	if err != nil {
		return err
	}
	return c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: util.ClusterToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("NomadCluster")),
		},
		// v1alpha4
		//handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("NomadCluster"))),
		predicates.ClusterUnpaused(r.Log),
	)
}

//v1alpha4
//func (r *NomadClusterReconciler) controlPlaneMachineToCluster(o client.Object) []ctrl.Request {

// controlPlaneMachineToCluster is a handler.ToRequestsFunc to be used
// to enqueue requests for reconciliation for NomadCluster to update
// its status.apiEndpoints field.
func (r *NomadClusterReconciler) controlPlaneMachineToCluster(o handler.MapObject) []ctrl.Request {
	// v1alpha4
	//nomadMachine, ok := o.(*infrav1.NomadMachine)

	nomadMachine, ok := o.Object.(*infrav1.NomadMachine)
	if !ok {
		r.Log.Error(nil, fmt.Sprintf("expected a NomadMachine but got a %T", o))
		return nil
	}
	if !IsControlPlaneMachine(nomadMachine) {
		return nil
	}

	ctx := context.TODO()

	// Fetch the CAPI Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, nomadMachine.ObjectMeta)
	if err != nil {
		r.Log.Error(err, "NomadMachine is missing cluster label or cluster does not exist",
			"namespace", nomadMachine.Namespace, "name", nomadMachine.Name)
		return nil
	}

	// Fetch the NomadCluster
	nomadCluster := &infrav1.NomadCluster{}
	nomadClusterKey := client.ObjectKey{
		Namespace: nomadMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, nomadClusterKey, nomadCluster); err != nil {
		r.Log.Error(err, "failed to get NomadCluster",
			"namespace", nomadClusterKey.Namespace, "name", nomadClusterKey.Name)
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: nomadClusterKey.Namespace,
			Name:      nomadClusterKey.Name,
		},
	}}
}
