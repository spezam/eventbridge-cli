package main

// EventBus --> EventBrige Rule --> SQS <-- poller

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
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
		Version: "1.1.0",
		Usage:   "AWS EventBridge cli",
		Authors: []*cli.Author{
			{Name: "matteo ridolfi", Email: "spezam@gmail.com"},
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
	awsCfg, err := newAWSConfig(c.String("profile"), c.String("region"))
	if err != nil {
		return err
	}

	// eventbridge client
	log.Printf("creating eventBridge client for bus [%s]", c.String("eventbusname"))
	ebClient := newEventbridgeClient(awsCfg, c.String("eventbusname"))

	// create temporary eventbridge event rule
	eventpattern := c.String("eventpattern")
	if strings.HasPrefix(eventpattern, "file://") {
		eventpattern, err = dataFromFile(eventpattern)
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
		log.Printf("removing EventBus target...")
		_ = ebClient.removeTarget(c.Context)

		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		_ = sqsClient.deleteQueue(c.Context)

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		_ = ebClient.deleteRule(c.Context)
	}()

	// switch between CI and standard modes
	switch c.Command.Name {
	case "ci":
		log.Printf("CI mode")

		// channels
		signalChan := make(chan os.Signal, 1) // SQS polling
		timedOut := make(chan struct{}, 1)    // timeout
		cleanupDone := make(chan struct{})    // resources cleanup
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

			// put event
			//time.Sleep(2 * time.Second) // might be needed if too fast
			if err := ebClient.putEvent(c.Context, event); err != nil {
				return err
			}
		}

		go func() {
			select {
			case <-time.After(time.Duration(c.Int64("timeout")) * time.Second):
				log.Printf("%d seconds timeout reached", c.Int64("timeout"))

				signalChan <- os.Interrupt
				timedOut <- struct{}{}

			case <-signalChan:
				log.Printf("message received")

				close(timedOut)
			}

			cleanupDone <- struct{}{}
		}()
		<-cleanupDone

		// check if CI timed out
		select {
		case _, ok := <-timedOut:
			if ok {
				return fmt.Errorf("CI failed - didn't receive any event")
			}

			log.Printf("CI successful")
			return nil
		}

	default:
		signalChan := make(chan os.Signal)
		signal.Notify(signalChan, os.Interrupt)
		// poll SQS queue undefinitely
		go sqsClient.pollQueue(c.Context, signalChan, c.Bool("prettyjson"))

		// wait for a SIGINT (ie. CTRL-C)
		<-signalChan
		log.Printf("received an interrupt, cleaning up...")
	}

	return nil
}

func newAWSConfig(profile, region string) (aws.Config, error) {
	external.DefaultSharedConfigProfile = profile
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return aws.Config{}, err
	}

	// override profile region if present
	if region != "" {
		cfg.Region = region
	}

	return cfg, nil
}

func dataFromFile(path string) (string, error) {
	f := strings.Replace(path, "file://", "", -1)
	e, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}

	return string(e), nil
}
