package main

// EventBus --> EventBrige Rule --> SQS <-- poller

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
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
		Version: "1.6.0",
		Usage:   "AWS EventBridge cli",
		Authors: []*cli.Author{
			{Name: "matteo ridolfi"},
		},
		Action:   run,
		Flags:    flags,
		Commands: commands,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(c *cli.Context) error {
	// AWS config
	awsCfg, err := newAWSConfig(c.Context, c.String("profile"), c.String("region"))
	if err != nil {
		return err
	}

	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", c.String("eventbusname"))
	ebClient := newEventbridgeClient(awsCfg, c.String("eventbusname"))

	// create temporary eventbridge event rule
	eventpattern := c.String("eventpattern")
	switch {
	case strings.HasPrefix(eventpattern, "file://"):
		eventpattern, err = dataFromFile(eventpattern)
		if err != nil {
			return err
		}

	case strings.HasPrefix(eventpattern, "sam://"):
		eventpattern, err = dataFromSAM(eventpattern)
		if err != nil {
			return err
		}
	}

	log.Printf("creating temporary rule on bus [%s]: %s", ebClient.eventBusName, eventpattern)
	ruleArn, err := ebClient.createRule(c.Context, eventpattern)
	if err != nil {
		return err
	}
	log.Printf("created temporary rule on bus [%s] with arn: %s", ebClient.eventBusName, ruleArn)

	// SQS client
	accountID := strings.Split(ruleArn, ":")[4]
	queueName := namespace + "-" + runID
	sqsClient := newSQSClient(awsCfg, accountID, queueName)

	// SQS queue
	if err := sqsClient.createQueue(c.Context, ruleArn); err != nil {
		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		_ = ebClient.deleteRule(c.Context)

		return err
	}
	log.Printf("created temporary SQS queue with URL: %s", sqsClient.queueURL)

	// EventBus --> SQS
	if err := ebClient.putTarget(c.Context, sqsClient.arn); err != nil {
		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		_ = sqsClient.deleteQueue(c.Context)

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		_ = ebClient.deleteRule(c.Context)

		return err
	}
	log.Printf("linked EventBus --> SQS...")

	// defer cleanup resources
	defer func() {
		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		_ = sqsClient.deleteQueue(c.Context)

		log.Printf("removing EventBus target...")
		_ = ebClient.removeTarget(c.Context)

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		_ = ebClient.deleteRule(c.Context)
	}()

	// switch between CI and standard modes
	switch c.Command.Name {
	case "ci":
		log.Printf("CI mode")

		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt)
		// poll SQS queue undefinitely
		go sqsClient.pollQueueCI(c.Context, signalChan, c.Bool("prettyjson"), c.Int64("timeout"))

		// read input event from cli or file
		event := c.String("inputevent")
		if event != "" {
			if strings.HasPrefix(event, "file://") {
				event, err = dataFromFile(event)
				if err != nil {
					return err
				}
			}

			//time.Sleep(2 * time.Second) // might be needed if too fast
			// put event
			if err := ebClient.putEvent(c.Context, event); err != nil {
				return err
			}
		}

		select {
		case <-time.After(time.Duration(c.Int64("timeout")) * time.Second):
			log.Printf("%d seconds timeout reached", c.Int64("timeout"))

			signalChan <- os.Interrupt
			return fmt.Errorf("CI failed - didn't receive any event")

		case <-signalChan:
			log.Printf("CI successful - message received")
			return nil
		}

	default:
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt)
		// poll SQS queue undefinitely
		go sqsClient.pollQueue(c.Context, signalChan, c.Bool("prettyjson"))

		// wait for a SIGINT (ie. CTRL-C)
		<-signalChan
		log.Printf("received an interrupt, cleaning up...")
	}

	return nil
}

func runTestEventPattern(c *cli.Context) error {
	// AWS config
	awsCfg, err := newAWSConfig(c.Context, c.String("profile"), c.String("region"))
	if err != nil {
		return err
	}

	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", c.String("eventbusname"))
	ebClient := newEventbridgeClient(awsCfg, c.String("eventbusname"))

	inputevent := c.String("inputevent")
	if strings.HasPrefix(inputevent, "file://") {
		inputevent, err = dataFromFile(inputevent)
		if err != nil {
			return err
		}
	}

	err = ebClient.testEventPattern(c.Context, inputevent, c.String("eventrule"))
	if err != nil {
		return err
	}

	return nil
}

func newAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	var awsCfg aws.Config
	var err error

	// use profile if present as cli parameter
	if profile != "" {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx)
	}
	if err != nil {
		return awsCfg, err
	}

	// check credentials validity
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		return awsCfg, err
	}

	// override profile region if present
	if region != "" {
		awsCfg.Region = region
	}

	return awsCfg, nil
}
