package dns

import (
	"context"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/consulclient"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/nomad/scope"
)

type Service struct {
	scope        *scope.MachineScope
	consulClient *consulclient.Client
}

// NewService returns a new helper for managing a Nomad "DNS" (DNS client loadbalancing)
func NewService(machineScope *scope.MachineScope) *Service {
	return &Service{
		scope:        machineScope,
		consulClient: scope.NewConsulClient(machineScope.ClusterScope),
	}
}

// RegisterService reconciles the load balancers for the given cluster.
func (s *Service) RegisterService() error {
	s.scope.V(2).Info("Register Service DNS")

	ctx := context.TODO()

	m := s.scope.Machine
	c := s.scope.ClusterScope

	service := &consulclient.Service{
		Name:    c.Cluster.Name,
		Id:      m.Name,
		Tags:    []string{"api-server"},
		Address: s.scope.GetDNSName(),
		Port:    6443,
	}

	return s.consulClient.RegisterService(ctx, service)
}

// DeregisterService reconciles the load balancers for the given cluster.
func (s *Service) DeregisterService() error {
	s.scope.V(2).Info("Deregister service")
	ctx := context.TODO()
	return s.consulClient.DeregisterService(ctx, s.scope.Machine.Name)
}
