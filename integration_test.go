// +build integration

package main

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func Test_integration(t *testing.T) {
	tests := []struct {
		name string

		eventbusname string
		eventpattern string
		inputevent   string

		err bool
	}{
		{
			name:         "successfull",
			eventbusname: "default",
			eventpattern: "file://testdata/eventpattern.json",
			inputevent:   "file://testdata/event_ci_success.json",
			err:          false,
		},
		{
			name:         "successfull from sam",
			eventbusname: "default",
			eventpattern: "sam://testdata/template.yaml/BetaFunction",
			inputevent:   "file://testdata/event_ci_success.json",
			err:          false,
		},
		{
			name:         "failing",
			eventbusname: "default",
			eventpattern: "file://testdata/eventpattern.json",
			inputevent:   "file://testdata/event_ci_fail.json",
			err:          true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// override global runID as the value doesn't refresh for each iteration
			runID = uuid.New().String()

			app := &cli.Command{
				Name:     namespace,
				Action:   run,
				Flags:    flags,
				Commands: commands,
			}

			err := app.Run(context.Background(), []string{
				namespace,
				"ci",
				"--eventbusname", test.eventbusname,
				"--eventpattern", test.eventpattern,
				"--inputevent", test.inputevent,
				"--prettyjson",
				"--timeout", "8",
			})

			if test.err {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
