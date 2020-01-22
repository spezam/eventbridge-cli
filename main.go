package main

// EventBus --> EventBrige Rule --> SQS <-- poller

import (
	"log"
	"os"
	"time"

	"github.com/urfave/cli/v2"
)

const namespace = "eventbridge-cli"

var unixTimeNow = time.Now().Unix()

func main() {
	app := &cli.App{
		Name:    "eventbridge-cli",
		Version: "0.0.1",
		Usage:   "AWS Eventbridge cli",
		Authors: []*cli.Author{
			&cli.Author{Name: "matteo ridolfi"},
		},
		Action: run,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "eventbusname",
				Aliases: []string{"b"},
				Usage:   "EventBridge Bus Name",
				Value:   "default",
			},
			&cli.BoolFlag{
				Name:    "prettyjson",
				Aliases: []string{"j"},
				Usage:   "Pretty JSON output",
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(c *cli.Context) error {
	log.Printf("eventbridge-cli")
	return nil
}
