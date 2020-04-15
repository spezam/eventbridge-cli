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
}
