// +build !integration

package main

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
)

var _ sqsClientAPI = (*mockSQSclient)(nil)

type mockSQSclient struct {
	err error

	queueURL        *string
	receiveMessages []types.Message
}

const (
	queueURL  = "http://localhost"
	arn       = "arn:aws:sqs:eu-north-1:1234567890:eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
	ruleArn   = "arn:aws:events:eu-north-1:1234567890:rule/eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
	queueName = "eventbridge-cli-14bc1c21-13ae-41a5-8951-76402ce2946e"
)

func init() {
	// disable logger
	log.SetOutput(ioutil.Discard)
}

func (m *mockSQSclient) CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
	return &sqs.CreateQueueOutput{
		QueueUrl: m.queueURL,
	}, m.err
}

func (m *mockSQSclient) DeleteQueue(ctx context.Context, params *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error) {
	return &sqs.DeleteQueueOutput{}, m.err
}

func (m *mockSQSclient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return &sqs.ReceiveMessageOutput{
		Messages: m.receiveMessages,
	}, m.err
}

func (m *mockSQSclient) DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	return &sqs.DeleteMessageBatchOutput{}, m.err
}

func Test_createQueue(t *testing.T) {
	tests := []struct {
		name string

		ruleArn string
		client  *mockSQSclient
		want    *sqsClient

		err bool
	}{
		{
			name:    "create SQS queue",
			ruleArn: ruleArn,
			client: &mockSQSclient{
				queueURL: aws.String(queueURL),
			},
			want: &sqsClient{
				client: &mockSQSclient{
					queueURL: aws.String(queueURL),
				},
				arn:       arn,
				queueName: queueName,
				queueURL:  queueURL,
			},
			err: false,
		},
		{
			name:    "create SQS queue error",
			ruleArn: ruleArn,
			client: &mockSQSclient{
				err: errors.New("unable to create SQS queue"),
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
				err: errors.New("unable to delete SQS queue"),
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
		signalChan := make(chan os.Signal, 1)

		go client.pollQueue(context.Background(), signalChan, false)
		signalChan <- os.Interrupt
	})
}

func Test_pollQueue(t *testing.T) {
	tests := []struct {
		name string

		client *mockSQSclient

		err bool
	}{
		{
			name: "poll SQS queue",
			client: &mockSQSclient{
				receiveMessages: []types.Message{
					{
						MessageId: aws.String("dc909f9a-377b-cc13-627d-6fdbc2ea458c"),
						Body:      aws.String(`{"detail-type":"Tag Change on Resource","source":"aws.tag"}`),
					},
				},
			},
			err: false,
		},
		{
			name: "poll SQS queue - no messages",
			client: &mockSQSclient{
				receiveMessages: []types.Message{},
			},
			err: false,
		},
		{
			name: "poll SQS queue - prettyJSON",
			client: &mockSQSclient{
				receiveMessages: []types.Message{
					{
						MessageId: aws.String("dc909f9a-377b-cc13-627d-6fdbc2ea458c"),
						Body:      aws.String(`{"detail-type":"Tag Change on Resource","source":"aws.tag"}`),
					},
				},
			},
			err: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signalChan := make(chan os.Signal, 1)
			client := &sqsClient{
				client:   test.client,
				queueURL: queueURL,
			}

			go client.pollQueue(context.Background(), signalChan, true)

			time.Sleep(2 * time.Second)
			signalChan <- os.Interrupt
		})
	}
}
