package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "profile",
		Aliases: []string{"p"},
		Usage:   "AWS profile",
		Value:   "default",
		EnvVars: []string{"AWS_PROFILE"},
	},
	&cli.StringFlag{
		Name:    "region",
		Aliases: []string{"r"},
		Usage:   "AWS region",
		EnvVars: []string{"AWS_REGION"},
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

var flagsCI = []cli.Flag{
	&cli.Int64Flag{
		Name:    "timeout",
		Aliases: []string{"t"},
		Usage:   "CI timeout in seconds",
		Value:   12,
	},
	&cli.StringFlag{
		Name:    "inputevent",
		Aliases: []string{"i"},
		Usage:   "Input event. If omitted expected from other sources",
		//Required: true,
	},
}
