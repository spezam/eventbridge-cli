package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/fatih/color"
)

type eventbridgeClient struct {
	client *eventbridge.Client

	eventBusName string
}

func newEventbridgeClient(cfg aws.Config, eventBusName string) *eventbridgeClient {
	return &eventbridgeClient{
		client:       eventbridge.NewFromConfig(cfg),
		eventBusName: eventBusName,
	}
}

func (e *eventbridgeClient) testEventPattern(ctx context.Context, inputEvent, eventRule string) error {
	// list eventbridge rules filtered by prefix
	resp, err := e.client.ListRules(ctx, &eventbridge.ListRulesInput{
		EventBusName: &e.eventBusName,
		NamePrefix:   aws.String(eventRule),
	})
	if err != nil {
		return err
	}

	if len(resp.Rules) < 1 {
		log.Printf("no event rule with prefix: %s", eventRule)
		return nil
	}

	log.Printf("event rules matching the event:")
	for _, r := range resp.Rules {
		// skip schedule rules since EventPattern is nil
		if r.EventPattern == nil {
			continue
		}

		res, err := e.client.TestEventPattern(ctx, &eventbridge.TestEventPatternInput{
			Event:        aws.String(inputEvent),
			EventPattern: r.EventPattern,
		})
		if err != nil {
			return err
		}

		if res.Result == false {
			log.Printf("%s: %s", *r.Name, color.RedString("✘"))
			continue
		}
		log.Printf("%s: %s", *r.Name, color.GreenString("✔"))
	}

	return nil
}

func (e *eventbridgeClient) createRule(ctx context.Context, eventPattern string) (string, error) {
	res, err := e.client.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(namespace + "-" + runID),
		Description:  aws.String(fmt.Sprintf("[%s] temp rule", namespace)),
		EventBusName: aws.String(e.eventBusName),
		EventPattern: aws.String(eventPattern),
		State:        types.RuleStateEnabled,
	})
	if err != nil {
		log.Printf("eventbridge.CreateRule error: %s", err)
		return "", err
	}

	return *res.RuleArn, nil
}

func (e *eventbridgeClient) deleteRule(ctx context.Context) error {
	_, err := e.client.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
		EventBusName: aws.String(e.eventBusName),
		Force:        true,
		Name:         aws.String(namespace + "-" + runID),
	})
	if err != nil {
		log.Printf("eventbridge.DeleteRule error: %s", err)
		return err
	}

	return err
}

func (e *eventbridgeClient) putEvent(ctx context.Context, event string) error {
	log.Printf("putting event: %s", event)
	ev := struct {
		Source     string `json:"source"`
		Detail     string `json:"detail"`
		DetailType string `json:"detail-type"`
	}{}
	err := json.Unmarshal([]byte(event), &ev)
	if err != nil {
		return err
	}

	resp, err := e.client.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []types.PutEventsRequestEntry{
			{
				Source:       aws.String(ev.Source),
				Detail:       aws.String(ev.Detail),
				DetailType:   aws.String(ev.DetailType),
				EventBusName: aws.String(e.eventBusName),
			},
		},
	})
	if err != nil {
		return err
	}

	if resp.FailedEntryCount > 0 {
		return fmt.Errorf("%s", *resp.Entries[0].ErrorMessage)
	}

	return nil
}

func (e *eventbridgeClient) putTarget(ctx context.Context, sqsArn string) error {
	_, err := e.client.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         aws.String(namespace + "-" + runID),
		EventBusName: aws.String(e.eventBusName),
		Targets: []types.Target{
			{
				Id:  aws.String(namespace + "-" + runID),
				Arn: aws.String(sqsArn),
			},
		},
	})
	if err != nil {
		log.Printf("eventbridge.PutTarget error %s", err)
		return err
	}

	return err
}

func (e *eventbridgeClient) removeTarget(ctx context.Context) error {
	_, err := e.client.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
		Ids: []string{
			namespace + "-" + runID,
		},
		Rule:         aws.String(namespace + "-" + runID),
		EventBusName: aws.String(e.eventBusName),
	})
	if err != nil {
		log.Printf("eventbridge.RemoveTarget error %s", err)
		return err
	}

	return err
}
