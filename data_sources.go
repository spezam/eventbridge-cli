package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
					Pattern      interface{} `yaml:"Pattern,omitempty"`
				} `yaml:"Properties"`
			} `yaml:"Events"`
		} `yaml:"Properties"`
	} `yaml:"Resources"`
}

// file://eventpattern.json
func dataFromFile(filepath string) (string, error) {
	file := strings.Replace(filepath, "file://", "", -1)

	e, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}

	return string(e), nil
}

// sam://template.yaml/FunctionName
func dataFromSAM(sampath string) (string, error) {
	function := path.Base(sampath)
	template := strings.Replace(sampath, "sam://", "", -1)
	template = strings.Replace(template, fmt.Sprintf("/%s", function), "", -1)

	e, err := ioutil.ReadFile(template)
	if err != nil {
		return "", err
	}

	// unmarshal SAM template
	b := &samTemplate{}
	if err := yaml.Unmarshal([]byte(e), &b); err != nil {
		return "", err
	}

	// find EventBridgeRule and marshal to JSON
	var p []byte
	for _, e := range b.Resources[function].Properties.Events {
		if e.Type == "EventBridgeRule" {
			if p, err = json.Marshal(convertMap(e.Properties.Pattern)); err != nil {
				return "", err
			}
		}
	}

	return string(p), nil
}

// convert map[interface{}]interface{} to map[string]interface{}
func convertMap(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m := map[string]interface{}{}
		for k, v := range x {
			m[k.(string)] = convertMap(v)
		}
		return m

	case []interface{}:
		for i, v := range x {
			x[i] = convertMap(v)
		}
	}

	return i
}
