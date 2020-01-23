package main

// EventBus --> EventBrige Rule --> SQS <-- poller

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

var (
	namespace = "eventbridge-cli"
	runID     = uuid.New().String()
)

func main() {
	app := &cli.App{
		Name:    namespace,
		Version: "0.1.0",
		Usage:   "AWS EventBridge cli",
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
			&cli.StringFlag{
				Name:    "eventpattern",
				Aliases: []string{"e"},
				Usage:   "EventBridge event pattern",
				Value:   fmt.Sprintf(`{"source": [{"anything-but": ["%s"]}]}`, namespace),
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
	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", c.String("eventbusname"))
	ebClient, err := newEventbridgeClient(c.String("eventbusname"))
	if err != nil {
		return err
	}

	// create temporary eventbridge event rule
	log.Printf("creating temporary rule on bus [%s]: %s", ebClient.eventBusName, c.String("eventpattern"))
	ruleArn, err := ebClient.createRule(c.Context, c.String("eventpattern"))
	if err != nil {
		return err
	}
	log.Printf("created temporary rule on bus [%s] with arn: %s", ebClient.eventBusName, ruleArn)

	// SQS client
	sqsClient, err := newSQSClient(c.Context, ruleArn)
	if err != nil {
		return err
	}
	log.Printf("created temporary SQS queue with URL: %s", sqsClient.queueURL)

	// EventBus --> SQS
	err = ebClient.putTarget(c.Context, sqsClient.sqsArn)
	if err != nil {
		return err
	}
	log.Printf("linked EventBus to SQS...")

	// poll SQS queue undefinitely
	breaker := make(chan struct{})
	go sqsClient.pollQueue(c.Context, breaker, c.Bool("prettyjson"))

	// wait for a SIGINT (ie. CTRL-C)
	// run cleanup when signal is received
	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan struct{})
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		<-signalChan

		log.Printf("received an interrupt, cleaning up...")
		breaker <- struct{}{}

		log.Printf("removing EventBus target...")
		ebClient.removeTarget(c.Context)

		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		sqsClient.deleteQueue(c.Context)

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		ebClient.deleteRule(c.Context)

		close(cleanupDone)
	}()
	<-cleanupDone

	return nil
}
