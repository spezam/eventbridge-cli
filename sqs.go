package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/TylerBrock/colorjson"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
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
		Attributes: map[string]*string{
			"Policy": aws.String(fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Id": "%s",
				"Statement": [{
					"Sid": "Sid1579089564623",
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
			}`, runID, s.arn, ruleArn)),
		},
	})
	if err != nil {
		log.Printf("sqs.CreateQueue error: %s", err)
		return err
	}

	s.queueURL = *resp.QueueUrl
	return err
}

func (s *sqsClient) deleteQueue(ctx context.Context) error {
	_, err := s.client.DeleteQueue(ctx, &sqs.DeleteQueueInput{
		QueueUrl: aws.String(s.queueURL),
	})
	if err != nil {
		log.Printf("sqs.DeleteQueue error: %s", err)
		return err
	}

	return err
}

func (s *sqsClient) pollQueue(ctx context.Context, signalChan chan os.Signal, prettyJSON bool) {
	log.Printf("polling queue %s ...", s.queueURL)
	log.Printf("press ctr+c to stop")
	defer close(signalChan)

	for {
		// goroutine
		select {
		case <-signalChan:
			log.Printf("stopping poller...")
			return

		default:
			resp, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
				QueueUrl:              aws.String(s.queueURL),
				MaxNumberOfMessages:   aws.Int32(10),
				WaitTimeSeconds:       aws.Int32(5),
				MessageAttributeNames: []*string{aws.String("All")},
			})
			// handle recovery from 'dial tcp' errors
			if err != nil && strings.Contains(err.Error(), "dial tcp") {
				log.Printf("sqs.ReceiveMessage error: %s", err)

				// backoff
				time.Sleep(10 * time.Second)
				continue
			}
			// handle all other errors
			if err != nil {
				log.Printf("sqs.ReceiveMessage error: %s", err)
				return
			}

			if len(resp.Messages) == 0 {
				continue
			}

			entries := []*types.DeleteMessageBatchRequestEntry{}
			for _, m := range resp.Messages {
				entries = append(entries, &types.DeleteMessageBatchRequestEntry{
					Id:            m.MessageId,
					ReceiptHandle: m.ReceiptHandle,
				})

				if prettyJSON {
					var j map[string]interface{}
					err := json.Unmarshal([]byte(*m.Body), &j)
					if err != nil {
						return
					}

					f := colorjson.NewFormatter()
					f.Indent = 2
					pj, _ := f.Marshal(j)

					log.Println(string(pj))
					continue
				}

				log.Printf("%s", *m.Body)
			}

			// cleanup messages
			_, err = s.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
				QueueUrl: aws.String(s.queueURL),
				Entries:  entries,
			})
			if err != nil {
				log.Printf("sqs.DeleteMessageBatch error: %s", err)
			}
		}
	}
}

func (s *sqsClient) pollQueueCI(ctx context.Context, signalChan chan os.Signal, prettyJSON bool, timeout int64) {
	log.Printf("polling queue %s ...", s.queueURL)
	defer close(signalChan)

	for {
		// goroutine
		select {
		case <-signalChan:
			log.Printf("stopping poller...")
			return

		default:
			resp, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
				QueueUrl:              aws.String(s.queueURL),
				MaxNumberOfMessages:   aws.Int32(10),
				WaitTimeSeconds:       aws.Int32(5),
				MessageAttributeNames: []*string{aws.String("All")},
			})
			if err != nil {
				log.Printf("sqs.ReceiveMessage error: %s", err)
				return
			}

			if len(resp.Messages) == 0 {
				continue
			}

			entries := []*types.DeleteMessageBatchRequestEntry{}
			for _, m := range resp.Messages {
				entries = append(entries, &types.DeleteMessageBatchRequestEntry{
					Id:            m.MessageId,
					ReceiptHandle: m.ReceiptHandle,
				})

				if prettyJSON {
					var j map[string]interface{}
					err := json.Unmarshal([]byte(*m.Body), &j)
					if err != nil {
						return
					}

					f := colorjson.NewFormatter()
					f.Indent = 2
					pj, _ := f.Marshal(j)

					log.Printf("received event: %s", string(pj))
					continue
				}

				log.Printf("received event: %s", *m.Body)
			}

			// cleanup messages
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
}
