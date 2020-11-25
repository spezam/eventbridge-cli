package main

import "github.com/urfave/cli/v2"

var commands = []*cli.Command{
	{
		Name:        "ci",
		Usage:       "AWS EventBridge cli - CI mode",
		Description: "run eventbridge-cli in CI mode",
		Flags:       flagsCI,
		Action:      run,
	},
	{
		Name:        "test-event",
		Usage:       "AWS EventBridge test-event",
		Description: "run eventbridge-cli to test an event against a deployed event rule pattern",
		Flags:       flagsTestEventPattern,
		Action:      runTestEventPattern,
	},
}
