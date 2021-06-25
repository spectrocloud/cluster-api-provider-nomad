package consulclient

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestRegisterService(t *testing.T) {
	c := NewClient(os.Getenv("CONSUL_ENDPOINT"), os.Getenv("CONSUL_API_KEY"))

	ctx := context.Background()

	releaseService := func(serviceID string) {
		if serviceID != "" {
			err := c.DeregisterService(ctx, serviceID)
			assert.Nil(t, err)
		}
	}

	t.Run("register-test-service", func(t *testing.T) {

		service := &Service{
			Name:    "test123",
			Id:      "test123",
			Tags:    []string{"hello"},
			Address: "127.0.0.1",
			Port:    5480,
		}
		err := c.RegisterService(ctx, service)
		assert.Nil(t, err, "expecting nil error")

		releaseService(service.Id)

	})
}
