// +build !integration

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dataFromFile(t *testing.T) {
	t.Run("reads file content", func(t *testing.T) {
		f, err := os.CreateTemp("", "eventpattern-*.json")
		require.NoError(t, err)
		defer os.Remove(f.Name())

		content := `{"source":["aws.ec2"]}`
		_, err = f.WriteString(content)
		require.NoError(t, err)
		f.Close()

		got, err := dataFromFile("file://" + f.Name())
		assert.NoError(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		_, err := dataFromFile("file:///nonexistent/path/pattern.json")
		assert.Error(t, err)
	})
}

func Test_dataFromSAM(t *testing.T) {
	const samYAML = `
Resources:
  MyFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: MyFunction
      Events:
        MyEvent:
          Type: EventBridgeRule
          Properties:
            EventBusName: default
            Pattern:
              source:
                - aws.ec2
              detail-type:
                - EC2 Instance State-change Notification
`

	t.Run("extracts event pattern from SAM template", func(t *testing.T) {
		dir := t.TempDir()
		tmplPath := filepath.Join(dir, "template.yaml")
		err := os.WriteFile(tmplPath, []byte(samYAML), 0644)
		require.NoError(t, err)

		got, err := dataFromSAM("sam://" + tmplPath + "/MyFunction")
		assert.NoError(t, err)
		assert.Contains(t, got, "aws.ec2")
		assert.Contains(t, got, "EC2 Instance State-change Notification")
	})

	t.Run("missing template file returns error", func(t *testing.T) {
		_, err := dataFromSAM("sam:///nonexistent/template.yaml/MyFunction")
		assert.Error(t, err)
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		dir := t.TempDir()
		tmplPath := filepath.Join(dir, "template.yaml")
		err := os.WriteFile(tmplPath, []byte("key: [unclosed"), 0644)
		require.NoError(t, err)

		_, err = dataFromSAM("sam://" + tmplPath + "/MyFunction")
		assert.Error(t, err)
	})
}

func Test_convertMap(t *testing.T) {
	t.Run("converts map[any]any to map[string]any", func(t *testing.T) {
		input := map[any]any{
			"key": "value",
		}
		result := convertMap(input)
		m, ok := result.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, "value", m["key"])
	})

	t.Run("converts nested maps recursively", func(t *testing.T) {
		input := map[any]any{
			"outer": map[any]any{
				"inner": "value",
			},
		}
		result := convertMap(input)
		m := result.(map[string]any)
		inner := m["outer"].(map[string]any)
		assert.Equal(t, "value", inner["inner"])
	})

	t.Run("converts []any elements recursively", func(t *testing.T) {
		input := []any{
			map[any]any{"key": "value"},
		}
		result := convertMap(input)
		slice := result.([]any)
		m := slice[0].(map[string]any)
		assert.Equal(t, "value", m["key"])
	})

	t.Run("passes through non-map types unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", convertMap("hello"))
		assert.Equal(t, 42, convertMap(42))
	})
}
