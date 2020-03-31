package main

// EventBus --> EventBrige Rule --> SQS <-- poller

import (
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"

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
			&cli.Author{Name: "matteo ridolfi"},
		},
		Action: run,
		Flags:  flags,
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
	if strings.HasPrefix(c.String("eventpattern"), "file://") {
		eventpattern, err = fromFile(c.String("eventpattern"))
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

func fromFile(path string) (string, error) {
	f := strings.Replace(path, "file://", "", -1)
	e, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}

	return string(e), nil
}
