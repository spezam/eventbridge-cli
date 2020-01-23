package main

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/sqsiface"
	"github.com/stretchr/testify/assert"
)

type mockSQSclient struct {
	sqsiface.ClientAPI

	createQueueResponse *sqs.CreateQueueOutput
	createQueueError    error
	deleteQueueError    error
}

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
		},
	}
}

func (m *mockSQSclient) DeleteQueueRequest(input *sqs.DeleteQueueInput) sqs.DeleteQueueRequest {
	return sqs.DeleteQueueRequest{
		Request: &aws.Request{
			Data:        &sqs.DeleteQueueOutput{},
			Error:       m.deleteQueueError,
			HTTPRequest: &http.Request{},
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
			ruleArn: "arn:aws:events:eu-north-1:1234567890:rule/eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e",
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
				queueURL: "https://localhost",
				sqsArn:   "arn:aws:sqs:eu-north-1:1234567890:eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e",
			},
			err: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &sqsClient{
				client: test.client,
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
				sqsArn:   "arn:aws:sqs:region:1234567890:eventbridge-cli-9be17b1e-b374-4a98-a0f4-1a4879153baf",
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

func Test_pollQueue(t *testing.T) {
	client := sqsClient{}
	breaker := make(chan struct{})
	tests := []struct {
		name string
		err  bool
	}{
		{
			name: "stopping poller",
			err:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			go client.pollQueue(ctx, breaker, false)
			breaker <- struct{}{}
			// assert.NoError(t, err)
		})
	}
}
