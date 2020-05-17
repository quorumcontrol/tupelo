package main_test

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	main "github.com/quorumcontrol/tupelo/aggregator/api/handler"
	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		request events.APIGatewayProxyRequest
		expect  string
		context context.Context
		err     error
	}{
		{
			// Test that the handler responds with the correct response
			// when a valid name is provided in the HTTP body
			request: events.APIGatewayProxyRequest{
				Body: `{"query":"query {\n  resolve(input:{did:\"test\",path:\"test\"}) {\n    remainingPath\n  }\n}","variables":null}`,
			},
			expect:  "{\"data\":{\"resolve\":{\"remainingPath\":[\"test\"]}}}",
			context: ctx,
			err:     nil,
		},
	}

	for _, test := range tests {
		response, err := main.Handler(test.context, test.request)
		assert.IsType(t, test.err, err)
		assert.Equal(t, test.expect, response.Body)
	}
}
