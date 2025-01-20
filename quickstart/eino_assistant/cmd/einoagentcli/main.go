/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino-examples/quickstart/eino_assistant/eino/einoagent"
	"github.com/cloudwego/eino-examples/quickstart/eino_assistant/pkg/mem"
)

var id = flag.String("id", "", "conversation id")

var memory = mem.GetDefaultMemory()

var cbHandler callbacks.Handler

func main() {
	flag.Parse()

	// 开启 Eino 的可视化调试能力
	err := devops.Init(context.Background())
	if err != nil {
		log.Printf("[eino dev] init failed, err=%v", err)
		return
	}

	if *id == "" {
		*id = strconv.Itoa(rand.Intn(1000000))
	}

	ctx := context.Background()

	err = Init()
	if err != nil {
		log.Printf("[eino agent] init failed, err=%v", err)
		return
	}

	// Start interactive dialogue
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("🧑‍ : ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			return
		}

		input = strings.TrimSpace(input)
		if input == "" || input == "exit" || input == "quit" {
			return
		}

		// Call RunAgent with the input
		sr, err := RunAgent(ctx, *id, input)
		if err != nil {
			fmt.Printf("Error from RunAgent: %v\n", err)
			continue
		}

		// Print the response
		fmt.Print("🤖 : ")
		for {
			msg, err := sr.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Printf("Error receiving message: %v\n", err)
				break
			}
			fmt.Print(msg.Content)
		}
		fmt.Println()
		fmt.Println()
	}
}

func Init() error {

	os.MkdirAll("log", 0755)
	var f *os.File
	f, err := os.OpenFile("log/eino.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	cbConfig := &LogCallbackConfig{
		Detail: true,
		Writer: f,
	}
	if os.Getenv("DEBUG") == "true" {
		cbConfig.Debug = true
	}
	// this is for invoke option of WithCallback
	cbHandler = LogCallback(cbConfig)

	// init global callback, for trace and metrics
	if os.Getenv("LANGFUSE_PUBLIC_KEY") != "" && os.Getenv("LANGFUSE_SECRET_KEY") != "" {
		fmt.Println("[eino agent] INFO: use langfuse as callback, watch at: https://cloud.langfuse.com")
		cbh, _ := langfuse.NewLangfuseHandler(&langfuse.Config{
			Host:      "https://cloud.langfuse.com",
			PublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
			SecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
			Name:      "Eino Assistant",
			Public:    true,
			Release:   "release/v0.0.1",
			UserID:    "eino_god",
			Tags:      []string{"eino", "assistant"},
		})
		callbacks.InitCallbackHandlers([]callbacks.Handler{cbh})
	}

	return nil
}

func RunAgent(ctx context.Context, id string, msg string) (*schema.StreamReader[*schema.Message], error) {

	runner, err := einoagent.BuildEinoAgent(ctx, &einoagent.BuildConfig{
		EinoAgent: &einoagent.EinoAgentBuildConfig{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build agent graph: %w", err)
	}

	conversation := memory.GetConversation(id, true)

	userMessage := &einoagent.UserMessage{
		ID:      id,
		Query:   msg,
		History: conversation.GetMessages(),
	}

	sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(cbHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to stream: %w", err)
	}

	srs := sr.Copy(2)

	go func() {
		// for save to memory
		fullMsgs := make([]*schema.Message, 0)

		defer func() {
			// close stream if you used it
			srs[1].Close()

			// add user input to history
			conversation.Append(schema.UserMessage(msg))

			fullMsg, err := schema.ConcatMessages(fullMsgs)
			if err != nil {
				fmt.Println("error concatenating messages: ", err.Error())
			}
			// add agent response to history
			conversation.Append(fullMsg)
		}()

	outer:
		for {
			select {
			case <-ctx.Done():
				fmt.Println("context done", ctx.Err())
				return
			default:
				chunk, err := srs[1].Recv()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break outer
					}
				}

				fullMsgs = append(fullMsgs, chunk)
			}
		}
	}()

	return srs[0], nil
}

type LogCallbackConfig struct {
	Detail bool
	Debug  bool
	Writer io.Writer
}

func LogCallback(config *LogCallbackConfig) callbacks.Handler {
	if config == nil {
		config = &LogCallbackConfig{
			Detail: true,
			Writer: os.Stdout,
		}
	}
	if config.Writer == nil {
		config.Writer = os.Stdout
	}
	builder := callbacks.NewHandlerBuilder()
	builder.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		fmt.Fprintf(config.Writer, "[view]: start [%s:%s:%s]\n", info.Component, info.Type, info.Name)
		if config.Detail {
			var b []byte
			if config.Debug {
				b, _ = json.MarshalIndent(input, "", "  ")
			} else {
				b, _ = json.Marshal(input)
			}
			fmt.Fprintf(config.Writer, "%s\n", string(b))
		}
		return ctx
	})
	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		fmt.Fprintf(config.Writer, "[view]: end [%s:%s:%s]\n", info.Component, info.Type, info.Name)
		return ctx
	})
	return builder.Build()
}
