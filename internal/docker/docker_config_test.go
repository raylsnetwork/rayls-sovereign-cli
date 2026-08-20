package docker

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetDemoComposeConfig(t *testing.T) {
	participants := []string{"a", "b"}
	composeConfig := GetDemoComposeConfig(participants, false, nil, false, nil, false, false, nil)

	yamlBytes, err := yaml.Marshal(composeConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	fmt.Println(string(yamlBytes))
}
