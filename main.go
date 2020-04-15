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
	err = sqsClient.createQueue(c.Context, ruleArn)
	if err != nil {
		return err
	}
	log.Printf("created temporary SQS queue with URL: %s", sqsClient.queueURL)

	// EventBus --> SQS
	err = ebClient.putTarget(c.Context, sqsClient.arn)
	if err != nil {
		return err
	}
	log.Printf("linked EventBus to SQS...")

	// switch between ci and standard modes
	switch c.Command.Name {
	case "ci":
		log.Printf("in ci mode")

		// poll SQS queue undefinitely
		signalChan := make(chan struct{})
		go sqsClient.pollQueueCI(c.Context, breaker, c.Bool("prettyjson"), c.Int64("timeout"))

		// put event
		time.Sleep(2 * time.Second)
		err = ebClient.putEvent(c.Context, c.String("event"))
		if err != nil {
			log.Printf("removing EventBus target...")
			ebClient.removeTarget(c.Context)

			log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
			sqsClient.deleteQueue(c.Context)

			log.Printf("deleting temporary EventBus rule %s...", ruleArn)
			ebClient.deleteRule(c.Context)

			return err
		}

		// wait for timeout or response
		done := make(chan bool)
		timedOut := make(chan bool)
		cleanupDone := make(chan struct{})
		go func() {
			select {
			case <-time.After(time.Duration(c.Int64("timeout")) * time.Second):
				log.Printf("%d seconds timeout reached", c.Int64("timeout"))
				log.Printf("removing EventBus target...")
				ebClient.removeTarget(c.Context)

				log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
				sqsClient.deleteQueue(c.Context)

				log.Printf("deleting temporary EventBus rule %s...", ruleArn)
				ebClient.deleteRule(c.Context)

				timedOut <- true

			case <-signalChan:
				log.Printf("message received")

				log.Printf("removing EventBus target...")
				ebClient.removeTarget(c.Context)

				log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
				sqsClient.deleteQueue(c.Context)

				log.Printf("deleting temporary EventBus rule %s...", ruleArn)
				ebClient.deleteRule(c.Context)

				done <- false
			}

			cleanupDone <- struct{}{}
		}()
		<-cleanupDone

		select {
		case <-done:
			return nil
		case <-timedOut:
			return fmt.Errorf("CI failed")
		}

		log.Printf("received an interrupt, cleaning up...")

		log.Printf("removing EventBus target...")
		ebClient.removeTarget(c.Context)

		log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
		sqsClient.deleteQueue(c.Context)

		log.Printf("deleting temporary EventBus rule %s...", ruleArn)
		ebClient.deleteRule(c.Context)

	default:
		// wait for a SIGINT (ie. CTRL-C)
		// run cleanup when signal is received
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt)
		// poll SQS queue undefinitely
		go sqsClient.pollQueue(c.Context, signalChan, c.Bool("prettyjson"))

		cleanupDone := make(chan struct{})
		go func() {
			<-signalChan

			log.Printf("received an interrupt, cleaning up...")

			log.Printf("removing EventBus target...")
			ebClient.removeTarget(c.Context)

			log.Printf("deleting temporary SQS queue %s...", sqsClient.queueURL)
			sqsClient.deleteQueue(c.Context)

			log.Printf("deleting temporary EventBus rule %s...", ruleArn)
			ebClient.deleteRule(c.Context)

			close(cleanupDone)
		}()
		<-cleanupDone
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
