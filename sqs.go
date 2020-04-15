package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/TylerBrock/colorjson"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/sqsiface"
)

type sqsClient struct {
	client sqsiface.ClientAPI

	arn       string
	queueName string
	queueURL  string
}

func newSQSClient(cfg aws.Config, accountID, queueName string) *sqsClient {
	return &sqsClient{
		client:    sqs.New(cfg),
		arn:       fmt.Sprintf("arn:aws:sqs:%s:%s:%s", cfg.Region, accountID, queueName),
		queueName: queueName,
	}
}

func (s *sqsClient) createQueue(ctx context.Context, ruleArn string) error {
	resp, err := s.client.CreateQueueRequest(&sqs.CreateQueueInput{
		QueueName: aws.String(s.queueName),
		Attributes: map[string]string{
			"Policy": fmt.Sprintf(`{
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
			}`, runID, s.arn, ruleArn),
		},
	}).Send(ctx)
	if err != nil {
		log.Printf("sqs.CreateQueue error: %s", err)
		return err
	}

	s.queueURL = *resp.QueueUrl
	return err
}

func (s *sqsClient) deleteQueue(ctx context.Context) error {
	_, err := s.client.DeleteQueueRequest(&sqs.DeleteQueueInput{
		QueueUrl: aws.String(s.queueURL),
	}).Send(ctx)
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
			resp, err := s.client.ReceiveMessageRequest(&sqs.ReceiveMessageInput{
				QueueUrl:              aws.String(s.queueURL),
				MaxNumberOfMessages:   aws.Int64(10),
				WaitTimeSeconds:       aws.Int64(5),
				MessageAttributeNames: []string{"All"},
			}).Send(ctx)
			// handle recovery from 'dial tcp' errors
			if err != nil && strings.Contains(err.Error(), "dial tcp") {
				log.Printf("sqs.ReceiveMessage error: %s", err)
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

			entries := []sqs.DeleteMessageBatchRequestEntry{}
			for _, m := range resp.Messages {
				entries = append(entries, sqs.DeleteMessageBatchRequestEntry{
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
			_, err = s.client.DeleteMessageBatchRequest(&sqs.DeleteMessageBatchInput{
				QueueUrl: aws.String(s.queueURL),
				Entries:  entries,
			}).Send(ctx)
			if err != nil {
				log.Printf("sqs.DeleteMessageBatch error: %s", err)
			}
		}
	}
}

func (s *sqsClient) pollQueueCI(ctx context.Context, breaker chan<- struct{}, prettyJSON bool, timeout int64) error {
	log.Printf("polling queue %s ...", s.queueURL)

	for {
		resp, err := s.client.ReceiveMessageRequest(&sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(s.queueURL),
			MaxNumberOfMessages:   aws.Int64(10),
			WaitTimeSeconds:       aws.Int64(5),
			MessageAttributeNames: []string{"All"},
		}).Send(ctx)
		if err != nil {
			log.Printf("sqs.ReceiveMessage error: %s", err)
			return err
		}

		if len(resp.Messages) == 0 {
			continue
		}

		entries := []sqs.DeleteMessageBatchRequestEntry{}
		for _, m := range resp.Messages {
			entries = append(entries, sqs.DeleteMessageBatchRequestEntry{
				Id:            m.MessageId,
				ReceiptHandle: m.ReceiptHandle,
			})

			if prettyJSON {
				var j map[string]interface{}
				err := json.Unmarshal([]byte(*m.Body), &j)
				if err != nil {
					return err
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
		_, err = s.client.DeleteMessageBatchRequest(&sqs.DeleteMessageBatchInput{
			QueueUrl: aws.String(s.queueURL),
			Entries:  entries,
		}).Send(ctx)
		if err != nil {
			log.Printf("sqs.DeleteMessageBatch error: %s", err)
		}

		breaker <- struct{}{}
	}
}
