package main

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func Test_convertMessagesForDeepSeek(t *testing.T) {
	msgs := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "try this.",
			ToolCalls: []schema.ToolCall{
				{
					ID:   "1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "func1",
						Arguments: "arguments1",
					},
				},
				{
					ID:   "2",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "func2",
						Arguments: "arguments2",
					},
				},
			},
		},
		{
			Role:    schema.Tool,
			Content: "tool content",
		},
	}

	converted := convertMessagesForDeepSeek(msgs)
	assert.Equal(t, []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "try this. call func1 with arguments1.  call func2 with arguments2. ",
		},
		{
			Role:    schema.User,
			Content: "",
		},
		{
			Role:    schema.Assistant,
			Content: "tool content",
		},
		{
			Role:    schema.User,
			Content: "",
		},
	}, converted)
}
