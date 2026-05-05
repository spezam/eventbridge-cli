package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/neilotoole/jsoncolor"
)

const (
	sqsMaxMessages  = 10
	sqsWaitSeconds  = 5
)

type sqsClient struct {
	client sqsClientAPI

	arn       string
	queueName string
	queueURL  string
}

type sqsClientAPI interface {
	CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	DeleteQueue(ctx context.Context, params *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

func newSQSClient(cfg aws.Config, accountID, queueName string) *sqsClient {
	return &sqsClient{
		client:    sqs.NewFromConfig(cfg),
		arn:       fmt.Sprintf("arn:aws:sqs:%s:%s:%s", cfg.Region, accountID, queueName),
		queueName: queueName,
	}
}

func (s *sqsClient) createQueue(ctx context.Context, ruleArn string) error {
	resp, err := s.client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(s.queueName),
		Attributes: map[string]string{
			"Policy": fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Id": "%s",
				"Statement": [{
					"Sid": "AllowEventBridgeToSendMessage-%s",
					"Effect": "Allow",
					"Principal": {
						"Service": "events.amazonaws.com"
					},
					"Action": "SQS:SendMessage",
					"Resource": "%s",
					"Condition": {
						"ArnEquals": {
							"aws:SourceArn": "%s"
						}
					}
				}]
			}`, s.queueName, s.queueName, s.arn, ruleArn),
			"SqsManagedSseEnabled": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("createQueue: %w", err)
	}

	s.queueURL = *resp.QueueUrl
	return nil
}

func (s *sqsClient) deleteQueue(ctx context.Context) error {
	_, err := s.client.DeleteQueue(ctx, &sqs.DeleteQueueInput{
		QueueUrl: aws.String(s.queueURL),
	})
	return err
}

func (s *sqsClient) pollQueue(ctx context.Context, doneChan chan struct{}, prettyJSON bool) {
	log.Printf("polling queue %s ...", s.queueURL)
	log.Printf("press ctrl+c to stop")
	defer close(doneChan)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(s.queueURL),
			MaxNumberOfMessages:   sqsMaxMessages,
			WaitTimeSeconds:       sqsWaitSeconds,
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) {
				log.Printf("sqs.ReceiveMessage error: %s", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
				}
				continue
			}
			log.Printf("sqs.ReceiveMessage error: %s", err)
			return
		}

		if len(resp.Messages) == 0 {
			continue
		}

		entries := make([]types.DeleteMessageBatchRequestEntry, 0, len(resp.Messages))
		for _, m := range resp.Messages {
			entries = append(entries, types.DeleteMessageBatchRequestEntry{
				Id:            m.MessageId,
				ReceiptHandle: m.ReceiptHandle,
			})

			if prettyJSON {
				log.Println(colorJSON(*m.Body))
				continue
			}

			log.Printf("%s", *m.Body)
		}

		_, err = s.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
			QueueUrl: aws.String(s.queueURL),
			Entries:  entries,
		})
		if err != nil {
			log.Printf("sqs.DeleteMessageBatch error: %s", err)
		}
	}
}

func (s *sqsClient) pollQueueCI(ctx context.Context, doneChan chan struct{}, readyChan chan struct{}, prettyJSON bool) {
	log.Printf("polling queue %s ...", s.queueURL)
	defer close(doneChan)
	close(readyChan)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(s.queueURL),
			MaxNumberOfMessages:   sqsMaxMessages,
			WaitTimeSeconds:       sqsWaitSeconds,
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) {
				log.Printf("sqs.ReceiveMessage error: %s", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
				}
				continue
			}
			log.Printf("sqs.ReceiveMessage error: %s", err)
			return
		}

		if len(resp.Messages) == 0 {
			continue
		}

		entries := make([]types.DeleteMessageBatchRequestEntry, 0, len(resp.Messages))
		for _, m := range resp.Messages {
			entries = append(entries, types.DeleteMessageBatchRequestEntry{
				Id:            m.MessageId,
				ReceiptHandle: m.ReceiptHandle,
			})

			if prettyJSON {
				log.Printf("received event: %s", colorJSON(*m.Body))
				continue
			}

			log.Printf("received event: %s", *m.Body)
		}

		_, err = s.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
			QueueUrl: aws.String(s.queueURL),
			Entries:  entries,
		})
		if err != nil {
			log.Printf("sqs.DeleteMessageBatch error: %s", err)
		}

		return
	}
}

func colorJSON(body string) string {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	buf := &bytes.Buffer{}
	enc := jsoncolor.NewEncoder(buf)
	enc.SetColors(jsoncolor.DefaultColors())
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return body
	}
	return buf.String()
}
