package main

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/sqsiface"
	"github.com/stretchr/testify/assert"
)

type mockSQSclient struct {
	sqsiface.ClientAPI

	createQueueResponse    *sqs.CreateQueueOutput
	receiveMessageResponse *sqs.ReceiveMessageOutput

	createQueueError    error
	deleteQueueError    error
	receiveMessageError error
}

const (
	arn       = "arn:aws:sqs:eu-north-1:1234567890:eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
	ruleArn   = "arn:aws:events:eu-north-1:1234567890:rule/eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
	queueName = "eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
)

func init() {
	// disable logger
	log.SetOutput(ioutil.Discard)
}

func (m *mockSQSclient) CreateQueueRequest(input *sqs.CreateQueueInput) sqs.CreateQueueRequest {
	return sqs.CreateQueueRequest{
		Request: &aws.Request{
			Data:        m.createQueueResponse,
			Error:       m.createQueueError,
			HTTPRequest: &http.Request{},
			Retryer:     aws.NoOpRetryer{},
		},
	}
}

func (m *mockSQSclient) DeleteQueueRequest(input *sqs.DeleteQueueInput) sqs.DeleteQueueRequest {
	return sqs.DeleteQueueRequest{
		Request: &aws.Request{
			Data:        &sqs.DeleteQueueOutput{},
			Error:       m.deleteQueueError,
			HTTPRequest: &http.Request{},
			Retryer:     aws.NoOpRetryer{},
		},
	}
}

func (m *mockSQSclient) ReceiveMessageRequest(input *sqs.ReceiveMessageInput) sqs.ReceiveMessageRequest {
	return sqs.ReceiveMessageRequest{
		Request: &aws.Request{
			Data:        m.receiveMessageResponse,
			Error:       m.receiveMessageError,
			HTTPRequest: &http.Request{},
			Retryer:     aws.NoOpRetryer{},
		},
	}
}

func (m *mockSQSclient) DeleteMessageBatchRequest(input *sqs.DeleteMessageBatchInput) sqs.DeleteMessageBatchRequest {
	return sqs.DeleteMessageBatchRequest{
		Request: &aws.Request{
			Data:        &sqs.DeleteMessageBatchOutput{},
			Error:       m.receiveMessageError,
			HTTPRequest: &http.Request{},
			Retryer:     aws.NoOpRetryer{},
		},
	}
}
func Test_createQueue(t *testing.T) {
	tests := []struct {
		name    string
		ruleArn string
		client  *mockSQSclient
		want    *sqsClient
		err     bool
	}{
		{
			name:    "create SQS queue",
			ruleArn: ruleArn,
			client: &mockSQSclient{
				createQueueResponse: &sqs.CreateQueueOutput{
					QueueUrl: aws.String("https://localhost"),
				},
			},
			want: &sqsClient{
				client: &mockSQSclient{
					createQueueResponse: &sqs.CreateQueueOutput{
						QueueUrl: aws.String("https://localhost"),
					},
				},
				arn:       arn,
				queueName: queueName,
				queueURL:  "https://localhost",
			},
			err: false,
		},
		{
			name:    "create SQS queue error",
			ruleArn: ruleArn,
			client: &mockSQSclient{
				createQueueError: errors.New("unable to create SQS queue"),
			},
			err: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &sqsClient{
				client:    test.client,
				arn:       arn,
				queueName: queueName,
			}

			err := client.createQueue(context.Background(), test.ruleArn)
			if test.err {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.want, client)
		})
	}
}

func Test_deleteQueue(t *testing.T) {
	tests := []struct {
		name   string
		client *mockSQSclient
		err    bool
	}{
		{
			name:   "delete SQS queue",
			client: &mockSQSclient{},
			err:    false,
		},
		{
			name: "delete SQS queue error",
			client: &mockSQSclient{
				deleteQueueError: errors.New("unable to delete SQS queue"),
			},
			err: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := sqsClient{
				client:   test.client,
				queueURL: "https://localhost",
				arn:      arn,
			}

			err := client.deleteQueue(context.Background())
			if test.err {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_pollQueueCancel(t *testing.T) {
	t.Run("stopping poller", func(t *testing.T) {
		client := sqsClient{}
		breaker := make(chan struct{})

		go client.pollQueue(context.Background(), breaker, false)
		breaker <- struct{}{}
	})
}

func Test_pollQueue(t *testing.T) {
	tests := []struct {
		name       string
		prettyJSON bool
		client     *mockSQSclient
		err        bool
	}{
		{
			name: "poll SQS queue",
			client: &mockSQSclient{
				receiveMessageResponse: &sqs.ReceiveMessageOutput{
					Messages: []sqs.Message{
						{
							MessageId: aws.String("dc909f9a-377b-cc13-627d-6fdbc2ea458c"),
							Body:      aws.String(`{"detail-type":"Tag Change on Resource","source":"aws.tag"}`),
						},
					},
				},
			},
			err: false,
		},
		{
			name: "poll SQS queue - no messages",
			client: &mockSQSclient{
				receiveMessageResponse: &sqs.ReceiveMessageOutput{
					Messages: []sqs.Message{},
				},
			},
			err: false,
		},
		{
			name: "poll SQS queue - prettyJSON",
			client: &mockSQSclient{
				receiveMessageResponse: &sqs.ReceiveMessageOutput{
					Messages: []sqs.Message{
						{
							MessageId: aws.String("dc909f9a-377b-cc13-627d-6fdbc2ea458c"),
							Body:      aws.String(`{"detail-type":"Tag Change on Resource","source":"aws.tag"}`),
						},
					},
				},
			},
			prettyJSON: true,
			err:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			breaker := make(chan struct{})
			client := &sqsClient{
				client:   test.client,
				queueURL: "https://localhost",
			}

			go client.pollQueue(context.Background(), breaker, test.prettyJSON)

			time.Sleep(2 * time.Second)
			breaker <- struct{}{}
		})
	}
}
