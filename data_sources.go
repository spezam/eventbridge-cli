package main

import (
	"encoding/json"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v2"
)

type samTemplate struct {
	Resources map[string]struct {
		Type       string `yaml:"Type"`
		Properties struct {
			FunctionName string `yaml:"FunctionName"`
			Events       map[string]struct {
				Type       string `yaml:"Type"`
				Properties struct {
					EventBusName string      `yaml:"EventBusName,omitempty"`
					InputPath    string      `yaml:"InputPath,omitempty"`
					Pattern      any `yaml:"Pattern,omitempty"`
				} `yaml:"Properties"`
			} `yaml:"Events"`
		} `yaml:"Properties"`
	} `yaml:"Resources"`
}

// file://eventpattern.json
func dataFromFile(filepath string) (string, error) {
	content, err := os.ReadFile(strings.TrimPrefix(filepath, "file://"))
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// sam://template.yaml/FunctionName
func dataFromSAM(sampath string) (string, error) {
	function := path.Base(sampath)
	template := strings.TrimSuffix(strings.TrimPrefix(sampath, "sam://"), "/"+function)

	content, err := os.ReadFile(template)
	if err != nil {
		return "", err
	}

	// unmarshal SAM template
	b := &samTemplate{}
	if err := yaml.Unmarshal(content, b); err != nil {
		return "", err
	}

	// find EventBridgeRule and marshal to JSON
	var pattern []byte
	for _, e := range b.Resources[function].Properties.Events {
		if e.Type == "EventBridgeRule" {
			if pattern, err = json.Marshal(convertMap(e.Properties.Pattern)); err != nil {
				return "", err
			}
		}
	}

	return string(pattern), nil
}

// convert map[any]any to map[string]any
func convertMap(i any) any {
	switch x := i.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, v := range x {
			m[k.(string)] = convertMap(v)
		}
		return m

	case []any:
		for i, v := range x {
			x[i] = convertMap(v)
		}
	}

	return i
}
