package replay

import (
	"os"

	"go.yaml.in/yaml/v3"
)

// sessionOf is the session id the stream names.
func sessionOf(stream []map[string]any) string {
	for _, f := range stream {
		if id, _ := f["session_id"].(string); id != "" {
			return id
		}
	}
	return ""
}

// recordedCommand is the command line run.yaml says the run was made with.
func recordedCommand(path string) (string, error) {
	var r struct {
		Command string `yaml:"command"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := yaml.Unmarshal(b, &r); err != nil {
		return "", err
	}
	return r.Command, nil
}
