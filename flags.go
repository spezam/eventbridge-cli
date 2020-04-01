package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/urfave/cli/v2"
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "profile",
		Aliases: []string{"p"},
		Usage:   "AWS profile",
		Value:   "default",
		EnvVars: []string{external.AWSProfileEnvVar},
	},
	&cli.StringFlag{
		Name:    "region",
		Aliases: []string{"r"},
		Usage:   "AWS region",
		EnvVars: []string{external.AWSDefaultRegionEnvVar},
	},
	&cli.StringFlag{
		Name:    "eventbusname",
		Aliases: []string{"b"},
		Usage:   "EventBridge Bus Name",
		Value:   "default",
	},
	&cli.StringFlag{
		Name:    "eventpattern",
		Aliases: []string{"e"},
		Usage:   "EventBridge event pattern. If prefixed with 'file://', a file will be used",
		Value:   fmt.Sprintf(`{"source": [{"anything-but": ["%s"]}]}`, namespace),
	},
	&cli.BoolFlag{
		Name:    "prettyjson",
		Aliases: []string{"j"},
		Usage:   "Pretty JSON output",
	},
}
