package machine

import (
	"fmt"
	"github.com/hashicorp/nomad/api"
	"github.com/pkg/errors"
	infrav1 "github.com/spectrocloud/cluster-api-provider-nomad/api/v1alpha3"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/scope"
	"hash/fnv"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"time"
)

//goland:noinspection HttpUrlsUsage
var (
	RepoURL = "http://imagerepo.sc"
)

// Service manages the Nomad machine
type Service struct {
	scope       *scope.MachineScope
	nomadClient *api.Client
}

// NewService DNS service returns a new helper for managing a Nomad "DNS" (DNS client loadbalancing)
func NewService(machineScope *scope.MachineScope) *Service {
	return &Service{
		scope:       machineScope,
		nomadClient: scope.NewNomadClient(machineScope.ClusterScope),
	}
}

//func (s *Service) GetMachine(systemID string) (*infrav1.Machine, error) {
//
//	m, err := s.nomadClient.Jobs().Info
//	if err != nil {
//		return nil, err
//	}
//
//	machine := fromSDKTypeToMachine(m)
//
//	return machine, nil
//}

func (s *Service) ReleaseMachine(jobID string) error {
	evalID, _, err := s.nomadClient.Jobs().Deregister(jobID, true, nil)
	if err != nil {
		return errors.Wrapf(err, "Unable to deregister job")
	}
	s.scope.Info("De-Registered job", "eval-id", evalID)
	return nil
}

func (s *Service) DeployMachine(userData string) (_ *infrav1.Machine, rerr error) {

	m := s.scope.Machine
	nm := s.scope.NomadMachine

	failureDomain := nm.Spec.FailureDomain
	if failureDomain == nil {
		failureDomain = s.scope.Machine.Spec.FailureDomain
	}

	//allocateOptions := &nomadclient.AllocateMachineOptions{
	//	AvailabilityZone: failureDomain,
	//
	//	ResourcePool: nm.Spec.ResourcePool,
	//	MinCPU:       nm.Spec.MinCPU,
	//	MinMem:       nm.Spec.MinMemory,
	//}

	userDataTemplate := &api.Template{
		DestPath:     pointer.StringPtr("local/boot/user-data"),
		EmbeddedTmpl: pointer.StringPtr(userData),

		// Unused (no string replacement)
		LeftDelim:  pointer.StringPtr("{{**{{"),
		RightDelim: pointer.StringPtr("}}**}}"),
	}

	metaDataTemplate := &api.Template{
		DestPath: pointer.StringPtr("local/boot/meta-data"),
		EmbeddedTmpl: pointer.StringPtr(fmt.Sprintf(
			"instance-id: %v\nlocal-hostname: %v",
			m.Name,
			m.Name,
		)),
	}

	qemuArgs := []string{
		"-nodefaults",
		"-machine", "type=q35",
		"-vga", "std",
		"-nic", fmt.Sprintf("tap,downscript=no,mac=%v", getMacAddress(m.Name)),
		"-drive", "file=fat:rw:/opt/nomad/data/alloc/${NOMAD_ALLOC_ID}/${NOMAD_TASK_NAME}/local/boot,file.label=cidata,format=raw,media=disk",
	}

	if nm.Spec.DebugVNC != nil {
		qemuArgs = append(qemuArgs, "-vnc", *nm.Spec.DebugVNC)
	}

	imageArtifact := &api.TaskArtifact{
		GetterSource: pointer.StringPtr(fmt.Sprintf("%v/%v.qcow2", RepoURL, nm.Spec.Image)),
	}

	t := &api.Task{
		Name:   "t1",
		Driver: "qemu",
		//User:            "",
		//Lifecycle:       nil,
		Config: map[string]interface{}{
			"image_path":  fmt.Sprintf("local/%v.qcow2", nm.Spec.Image),
			"accelerator": "kvm",
			"args":        qemuArgs,
		},
		//Constraints:     nil,
		//Affinities:      nil,
		//Env:             nil,
		//Services:        nil,
		Resources: &api.Resources{
			Cores:    &nm.Spec.CPU,
			MemoryMB: &nm.Spec.MemoryMB,
		},
		//RestartPolicy:   nil,
		//Meta:            nil,
		//KillTimeout:     nil,
		//LogConfig:       nil,
		//Vault:           nil,
		Artifacts:       []*api.TaskArtifact{imageArtifact},
		Templates:       []*api.Template{userDataTemplate, metaDataTemplate},
		DispatchPayload: nil,
		VolumeMounts:    nil,
		CSIPluginConfig: nil,
		Leader:          false,
		ShutdownDelay:   0,
		KillSignal:      "",
		Kind:            "",
		ScalingPolicies: nil,
	}

	tg := &api.TaskGroup{
		Name: pointer.StringPtr("g2"),
		//Count:                     nil,
		//Constraints:               nil,
		//Affinities:                nil,
		Tasks: []*api.Task{t},
		//Spreads:                   nil,
		//Volumes:                   nil,
		//RestartPolicy:             nil,
		//ReschedulePolicy:          nil,
		//EphemeralDisk:             nil,
		//Update:                    nil,
		//Migrate:                   nil,
		Networks: nil,
		//Meta:                      nil,
		Services: nil,
		//ShutdownDelay:             nil,
		//StopAfterClientDisconnect: nil,
		//Scaling:                   nil,
		//Consul:                    nil,
	}
	job := &api.Job{
		Region:    pointer.StringPtr("global"),
		Namespace: pointer.StringPtr("default"),
		// TODO
		Name: &m.Name,
		ID:   &m.Name,

		//Priority:          nil,
		//AllAtOnce:         nil,
		Datacenters: []string{*failureDomain},
		//Constraints:       nil,
		//Affinities:        nil,
		TaskGroups: []*api.TaskGroup{tg},
		//Update:            nil,
		//Multiregion:       nil,
		//Spreads:           nil,
		//Periodic:          nil,
		//ParameterizedJob:  nil,
		//Reschedule:        nil,
		//Migrate:           nil,
		//Meta:              nil,
		//ConsulToken:       nil,
		//VaultToken:        nil,
		//Stop:              nil,
		//ParentID:          nil,
		//Dispatched:        false,
		//Payload:           nil,
		//ConsulNamespace:   nil,
		//VaultNamespace:    nil,
		//NomadTokenID:      nil,
		//Status:            nil,
		//StatusDescription: nil,
		//Stable:            nil,
		//Version:           nil,
		//SubmitTime:        nil,
		//CreateIndex:       nil,
		//ModifyIndex:       nil,
		//JobModifyIndex:    nil,
	}
	j, _, err := s.nomadClient.Jobs().Register(job, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to register job")
	} else if j.EvalID == "" {
		return nil, errors.Wrapf(err, "No eval-id returned")
	}

	s.scope.Info("Registered job", "eval-id", j.EvalID)

	deploymentID, err := s.getDeploymentIDFromEval(j.EvalID)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to find matching deployment")
	}

	s.scope.Info("Deployment id", "deployment-id", deploymentID)
	d, err := s.getDeployment(deploymentID)
	return s.deploymentToMachine(d), nil

	//defer func() {
	//	if rerr != nil {
	//		s.scope.Info("Attempting to release machine which failed to deploy")
	//		err := s.nomadClient.ReleaseMachine(ctx, m.SystemID)
	//		if err != nil {
	//			// Is it right to NOT set rerr so we can see the original issue?
	//			log.Error(err, "Unable to release properly")
	//		}
	//	}
	//}()
	//
	// TODO need to revisit if we need to set the hostname OR not
	//Hostname: &nm.Name,
	//noSwap := 0
	//updateOptions := nomadclient.UpdateMachineOptions{
	//	SystemID: m.SystemID,
	//	SwapSize: &noSwap,
	//}
	//if _, err := s.nomadClient.UpdateMachine(ctx, updateOptions); err != nil {
	//	return nil, errors.Wrapf(err, "Unable to disable swap")
	//}

	//s.scope.Info("Swap disabled", "system-id", m.SystemID)
	//
	//deployOptions := nomadclient.DeployMachineOptions{
	//	SystemID:     m.SystemID,
	//	UserData:     pointer.StringPtr(userDataB64),
	//	OSSystem:     pointer.StringPtr("custom"),
	//	DistroSeries: &nm.Spec.Image,
	//}
	//
	//deployingM, err := s.nomadClient.DeployMachine(ctx, deployOptions)
	//if err != nil {
	//	return nil, errors.Wrapf(err, "Unable to deploy machine")
	//}

	//return fromSDKTypeToMachine(deployingM), nil
}

func (s *Service) getDeployment(deploymentID string) (*api.Deployment, error) {
	deployment, _, err := s.nomadClient.Deployments().Info(deploymentID, nil)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

func (s *Service) getDeploymentIDFromEval(evalID string) (string, error) {
	eval, _, err := s.nomadClient.Evaluations().Info(evalID, nil)
	if err != nil {
		return "", err
	}

	switch eval.Status {
	case "complete":
		return eval.DeploymentID, nil
	case "pending":
		time.Sleep(1 * time.Second)
		s.scope.Info("Eval is pending", "eval-id", evalID)
		return s.getDeploymentIDFromEval(evalID)
	}

	return "", errors.Errorf("Unable state %s for eval %s", eval.Status, eval.ID)
}

func (s *Service) deploymentToMachine(deployment *api.Deployment) *infrav1.Machine {
	address := clusterv1.MachineAddress{
		Type:    clusterv1.MachineExternalDNS,
		Address: s.scope.GetDNSName(),
	}
	machine := &infrav1.Machine{
		JobID:             deployment.JobID,
		DeploymentID:      deployment.ID,
		Status:            infrav1.MachineStatus(deployment.Status),
		StatusDescription: deployment.StatusDescription,
		Addresses:         []clusterv1.MachineAddress{address},
	}

	return machine
}

func hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func getMacAddress(name string) string {
	h := hash(name)
	mac := []interface{}{0x52, 0x54, 0x0, h % 211, h % 251, h % 256}

	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", mac...)
}

//	machine := &infrav1.Machine{
//		ID:               m.SystemID,
//		Hostname:         m.Hostname,
//		Status:            infrav1.MachineStatus(m.Status),
//		Powered:          m.PowerState == "on",
//		AvailabilityZone: m.AvailabilityZone,
//	}
//
//	if m.FQDN != "" {
//		machine.Addresses = append(machine.Addresses, clusterv1.MachineAddress{
//			Type:    clusterv1.MachineExternalDNS,
//			Address: m.FQDN,
//		})
//	}
//
//	for _, v := range m.IpAddresses {
//		machine.Addresses = append(machine.Addresses, clusterv1.MachineAddress{
//			Type:    clusterv1.MachineExternalIP,
//			Address: v,
//		})
//	}
//
//	return machine
//}

//// ReconcileDNS reconciles the load balancers for the given cluster.
//func (s *Service) ReconcileDNS() error {
//	s.scope.V(2).Info("Reconciling DNS")
//
//	s.scope.SetDNSName("cluster1.nomad")
//	return nil
//}
//
