package docker

import (
	"context"
	"encoding/json"
	"fmt"
)

func (c *Client) InspectContainer(ctx context.Context, name string) (string, error) {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	result := map[string]interface{}{
		"status":  inspect.State.Status,
		"ports":   inspect.NetworkSettings.Ports,
		"env":     inspect.Config.Env,
		"mounts":  inspect.Mounts,
		"network": inspect.NetworkSettings.Networks,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal container inspect: %w", err)
	}

	return string(data), nil
}
