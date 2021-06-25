package consulclient

import (
	"context"
	"fmt"
	"net/http"
)

type Service struct {
	Name    string   `json:"name"`
	Id      string   `json:"id"`
	Tags    []string `json:"tags"`
	Address string   `json:"address"`
	Port    int      `json:"port"`
}

func (c *Client) RegisterService(ctx context.Context, s *Service) error {

	if err := c.send(ctx, http.MethodPut, "/v1/agent/service/register", s, nil); err != nil {
		return err
	}

	return nil
}

func (c *Client) DeregisterService(ctx context.Context, serviceID string) error {

	if err := c.send(ctx, http.MethodPut, fmt.Sprintf("/v1/agent/service/deregister/%s", serviceID), nil, nil); err != nil {
		return err
	}

	return nil
}
