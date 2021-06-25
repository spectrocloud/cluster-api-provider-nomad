package v1alpha3

import (
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// MachineStatus describes the state of an AWS instance.
type MachineStatus string

// List of all possible states: https://github.com/nomad/nomad/blob/master/src/nomadserver/enum.py#L108

var (
	// MachineStateAllocated is the string representing an instance in a ready (commissioned) state
	//MachineStateAllocated = MachineStatus("Allocated")


	// MachineStateRunning is the string representing an instance in a pending state
	MachineStateRunning = MachineStatus("running")

	//MachineStateSuccessful is the state when it's DONE
	MachineStateSuccessful = MachineStatus("successful")

	// MachineStateReady is the string representing an instance in a ready (commissioned) state
	//MachineStateReady = MachineStatus("Ready")

	// MachineStateDiskErasing is the string representing an instance which is releasing (disk)
	//MachineStateDiskErasing = MachineStatus("Disk erasing")

	// MachineStateDiskErasing is the string representing an instance which is releasing
	//MachineStateReleasing = MachineStatus("Releasing")

	// MachineStateNew is the string representing an instance which is not yet commissioned
	//MachineStateNew = MachineStatus("New")

	//// MachineStateShuttingDown is the string representing an instance shutting down
	//MachineStateShuttingDown = MachineStatus("shutting-down")
	//
	//// MachineStateTerminated is the string representing an instance that has been terminated
	//MachineStateTerminated = MachineStatus("terminated")
	//
	//// MachineStateStopping is the string representing an instance
	//// that is in the process of being stopped and can be restarted
	//MachineStateStopping = MachineStatus("stopping")

	// MachineStateStopped is the string representing an instance
	// that has been stopped and can be restarted
	//MachineStateStopped = MachineStatus("stopped")

	// MachineRunningStates defines the set of states in which an Nomad instance is
	// running or going to be running soon
	MachineRunningStates = sets.NewString(
		string(MachineStateSuccessful),
		string(MachineStateRunning),
	)

	// MachineOperationalStates defines the set of states in which an Nomad instance is
	// or can return to running, and supports all Nomad operations
	MachineOperationalStates = MachineRunningStates.Union(
		sets.NewString(
		//string(MachineStateAllocated),
		),
	)

	// MachineKnownStates represents all known Nomad instance states
	MachineKnownStates = MachineOperationalStates.Union(
		sets.NewString(
		//string(MachineStateDiskErasing),
		//string(MachineStateReleasing),
		//string(MachineStateReady),
		//string(MachineStateNew),
		//string(MachineStateTerminated),
		),
	)
)

// Machine describes an Nomad deployment
type Machine struct {
	JobID        string
	DeploymentID string

	// Hostname is the hostname
	//Hostname string

	// Status The current state of the machine.
	Status MachineStatus

	// StatusDescription represents the state of the deployment
	StatusDescription string

	// The current state of the machine.
	//Powered bool

	// The AZ of the machine
	//AvailabilityZone string

	// Addresses contains the AWS instance associated addresses.
	Addresses []clusterv1.MachineAddress
}
