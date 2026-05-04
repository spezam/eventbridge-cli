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
	"github.com/urfave/cli/v3"
)

var (
	namespace = "eventbridge-cli"
	runID     = uuid.New().String()
)

func main() {
	app := &cli.Command{
		Name:    namespace,
		Version: "2.0.0",
		Usage:   "AWS EventBridge cli",
		Authors: []any{"matteo ridolfi"},
		Action:   run,
		Flags:    flags,
		Commands: commands,
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	// AWS config
	awsCfg, err := newAWSConfig(ctx, cmd.String("profile"), cmd.String("region"))
	if err != nil {
		return err
	}

	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", cmd.String("eventbusname"))
	ebClient := newEventbridgeClient(awsCfg, cmd.String("eventbusname"))

	// create temporary eventbridge event rule
	eventpattern := cmd.String("eventpattern")
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
	ruleArn, err := ebClient.createRule(ctx, eventpattern)
	if err != nil {
		return err
	}
	log.Printf("created temporary rule on bus [%s] with arn: %s", ebClient.eventBusName, ruleArn)

	// SQS client
	arnParts := strings.Split(ruleArn, ":")
	if len(arnParts) < 5 {
		return fmt.Errorf("unexpected rule ARN format: %s", ruleArn)
	}
	accountID := arnParts[4]
	queueName := namespace + "-" + runID
	sqsClient := newSQSClient(awsCfg, accountID, queueName)

	// SQS queue
	if err := sqsClient.createQueue(ctx, ruleArn); err != nil {
		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		if cleanupErr := ebClient.deleteRule(ctx); cleanupErr != nil {
			log.Printf("failed to delete EventBus rule %s: %v", ruleArn, cleanupErr)
		}
		return err
	}
	log.Printf("created temporary SQS queue with URL: %s", sqsClient.queueURL)

	// EventBus --> SQS
	if err := ebClient.putTarget(ctx, sqsClient.arn); err != nil {
		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		if cleanupErr := sqsClient.deleteQueue(ctx); cleanupErr != nil {
			log.Printf("failed to delete SQS queue %s: %v", sqsClient.queueURL, cleanupErr)
		}

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		if cleanupErr := ebClient.deleteRule(ctx); cleanupErr != nil {
			log.Printf("failed to delete EventBus rule %s: %v", ruleArn, cleanupErr)
		}

		return err
	}
	log.Printf("linked EventBus --> SQS...")

	// defer cleanup resources
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		if err := sqsClient.deleteQueue(cleanupCtx); err != nil {
			log.Printf("failed to delete SQS queue %s: %v", sqsClient.queueURL, err)
		}

		log.Printf("removing EventBus target...")
		if err := ebClient.removeTarget(cleanupCtx); err != nil {
			log.Printf("failed to remove EventBus target: %v", err)
		}

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		if err := ebClient.deleteRule(cleanupCtx); err != nil {
			log.Printf("failed to delete EventBus rule %s: %v", ruleArn, err)
		}
	}()

	// switch between CI and standard modes
	switch cmd.Name {
	case "ci":
		log.Printf("CI mode")

		pollCtx, cancelPoll := context.WithCancel(ctx)
		defer cancelPoll()

		signalChan := make(chan os.Signal, 1)
		doneChan := make(chan struct{})
		readyChan := make(chan struct{})
		signal.Notify(signalChan, os.Interrupt)
		defer signal.Stop(signalChan)
		// poll SQS queue until event received or timeout
		go sqsClient.pollQueueCI(pollCtx, signalChan, doneChan, readyChan, cmd.Bool("prettyjson"), cmd.Int64("timeout"))

		// wait for poller to start before sending the event
		<-readyChan

		// read input event from cli or file
		event := cmd.String("inputevent")
		if event != "" {
			if strings.HasPrefix(event, "file://") {
				event, err = dataFromFile(event)
				if err != nil {
					return err
				}
			}

			// put event
			if err := ebClient.putEvent(ctx, event); err != nil {
				return err
			}
		}

		select {
		case <-time.After(time.Duration(cmd.Int64("timeout")) * time.Second):
			log.Printf("%d seconds timeout reached", cmd.Int64("timeout"))
			return fmt.Errorf("CI failed - didn't receive any event")

		case <-doneChan:
			log.Printf("CI successful - message received")
			return nil

		case <-signalChan:
			return nil
		}

	default:
		signalChan := make(chan os.Signal, 1)
		doneChan := make(chan struct{})
		signal.Notify(signalChan, os.Interrupt)
		defer signal.Stop(signalChan)
		// poll SQS queue undefinitely
		go sqsClient.pollQueue(ctx, signalChan, doneChan, cmd.Bool("prettyjson"))

		// wait for a SIGINT (ie. CTRL-C) or poller exit
		select {
		case <-signalChan:
			log.Printf("received an interrupt, cleaning up...")
		case <-doneChan:
		}
	}

	return nil
}

func runTestEventPattern(ctx context.Context, cmd *cli.Command) error {
	// AWS config
	awsCfg, err := newAWSConfig(ctx, cmd.String("profile"), cmd.String("region"))
	if err != nil {
		return err
	}

	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", cmd.String("eventbusname"))
	ebClient := newEventbridgeClient(awsCfg, cmd.String("eventbusname"))

	inputevent := cmd.String("inputevent")
	if strings.HasPrefix(inputevent, "file://") {
		inputevent, err = dataFromFile(inputevent)
		if err != nil {
			return err
		}
	}

	err = ebClient.testEventPattern(ctx, inputevent, cmd.String("eventrule"))
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
