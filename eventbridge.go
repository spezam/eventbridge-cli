package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
)

type eventbridgeClient struct {
	client *eventbridge.Client

	eventBusName string
}

func newEventbridgeClient(eventBusName string) (*eventbridgeClient, error) {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, err
	}

	return &eventbridgeClient{
		client:       eventbridge.New(cfg),
		eventBusName: eventBusName,
	}, err
}

// create temp event rule
func (e *eventbridgeClient) createRule(ctx context.Context) (string, error) {
	res, err := e.client.PutRuleRequest(&eventbridge.PutRuleInput{
		Name:         aws.String(namespace + "-" + runID),
		Description:  aws.String(fmt.Sprintf("[%s] temp rule", namespace)),
		EventBusName: aws.String(e.eventBusName),
		EventPattern: aws.String(fmt.Sprintf(`{"source": [{"anything-but": ["%s"]}]}`, namespace)),
		State:        eventbridge.RuleStateEnabled,
	}).Send(ctx)
	if err != nil {
		log.Printf("eventbridge.CreateRule error: %s", err)
		return "", err
	}

	return *res.RuleArn, nil
}

func (e *eventbridgeClient) deleteRule(ctx context.Context) error {
	_, err := e.client.DeleteRuleRequest(&eventbridge.DeleteRuleInput{
		EventBusName: aws.String(e.eventBusName),
		Force:        aws.Bool(true),
		Name:         aws.String(namespace + "-" + runID),
	}).Send(ctx)
	if err != nil {
		log.Printf("eventbridge.DeleteRule error: %s", err)
		return err
	}

	return err
}

func (e *eventbridgeClient) putTarget(ctx context.Context, sqsArn string) error {
	_, err := e.client.PutTargetsRequest(&eventbridge.PutTargetsInput{
		Rule:         aws.String(namespace + "-" + runID),
		EventBusName: aws.String(e.eventBusName),
		Targets: []eventbridge.Target{
			eventbridge.Target{
				Id:  aws.String(namespace + "-" + runID),
				Arn: aws.String(sqsArn),
			},
		},
	}).Send(ctx)
	if err != nil {
		log.Printf("eventbridge.PutTarget error %s", err)
		return err
	}

	return err
}

func (e *eventbridgeClient) removeTarget(ctx context.Context) error {
	_, err := e.client.RemoveTargetsRequest(&eventbridge.RemoveTargetsInput{
		Ids: []string{
			namespace + "-" + runID,
		},
		Rule:         aws.String(namespace + "-" + runID),
		EventBusName: aws.String(e.eventBusName),
	}).Send(ctx)
	if err != nil {
		log.Printf("eventbridge.RemoveTarget error %s", err)
		return err
	}

	return err
}
