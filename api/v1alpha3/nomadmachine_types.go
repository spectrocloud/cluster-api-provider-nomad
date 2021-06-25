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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer allows NomadMachineReconciler to clean up resources associated with NomadMachine before
	// removing it from the apiserver.
	MachineFinalizer = "nomadcluster.infrastructure.cluster.x-k8s.io"
)

// NomadMachineSpec defines the desired state of NomadMachine
type NomadMachineSpec struct {

	// FailureDomain is the failure domain the machine will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	FailureDomain *string `json:"failureDomain,omitempty"`

	// ProviderID will be the name in ProviderID format (nomad:///clusterName/machineName)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// CPU minimum number of CPUs
	CPU int `json:"cpu"`

	// MinMemory minimum memory
	MemoryMB int `json:"memoryMB,omitempty"`

	// DebugVNC specifies whether we want a debugging console, e.g: ":1" (is port 5901)
	// +optional
	DebugVNC *string `json:"debugVNC,omitempty"`

	// Image will be the Nomad image id
	Image string `json:"image"`
}

// NomadMachineStatus defines the observed state of NomadMachine
type NomadMachineStatus struct {

	// Ready denotes that the machine (nomad container) is ready
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// EvalID will be the Nomad eval ID for the job
	// +optional
	//EvalID *string `json:"evalID,omitempty"`

	// DeploymentStatus is the state of the AWS instance for this machine.
	DeploymentStatus *MachineStatus `json:"deploymentStatus,omitempty"`

	// StatusDescription is the state of the AWS instance for this machine.
	StatusDescription *string `json:"statusDescription,omitempty"`

	// MachinePowered is if the machine is "Powered" on
	//MachinePowered bool `json:"machinePowered,omitempty"`

	//Hostname is the actual Nomad hostname
	//Hostname *string `json:"hostname,omitempty"`

	// DNSAttached specifies whether the DNS record contains the IP of this machine
	DNSAttached bool `json:"dnsAttached,omitempty"`

	// Addresses contains the associated addresses for the nomad machine.
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the NomadMachine.
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	FailureReason *errors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:resource:path=nomadmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// NomadMachine is the Schema for the nomadmachines API
type NomadMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NomadMachineSpec   `json:"spec,omitempty"`
	Status NomadMachineStatus `json:"status,omitempty"`
}

func (c *NomadMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *NomadMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// NomadMachineList contains a list of NomadMachine
type NomadMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NomadMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NomadMachine{}, &NomadMachineList{})
}
