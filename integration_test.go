// +build integration

package main

import (
	"flag"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v2"
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

			app := cli.NewApp()
			// flags
			set := flag.NewFlagSet("integration-test", 0)
			set.String("eventbusname", test.eventbusname, "")
			set.String("eventpattern", test.eventpattern, "")
			set.Bool("prettyjson", true, "")
			// ci flags
			set.Int64("timeout", 8, "")
			set.String("inputevent", test.inputevent, "")

			c := cli.NewContext(app, set, nil)
			c.Command.Name = "ci"
			err := run(c)
			if test.err {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
