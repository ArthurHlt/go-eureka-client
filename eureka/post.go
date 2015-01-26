package eureka

import (
	"encoding/json"
	"strings"
)

func (c *Client) RegisterInstance(appId string, instanceInfo *InstanceInfo) error {
	values := []string{"apps", appId}
	path := strings.Join(values, "/")
	body, err := json.Marshal(instanceInfo)
	if err != nil {
		return err
	}

	_, err = c.post(path, body)
	return err
}
