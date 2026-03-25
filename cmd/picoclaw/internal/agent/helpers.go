package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ergochat/readline"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func agentCmd(message, sessionKey, model string, debug bool) error {
	if sessionKey == "" {
		sessionKey = "cli:default"
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	}

	if model != "" {
		cfg.Agents.Defaults.ModelName = model
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
	defer agentLoop.Close()

	// Print agent startup info (only for interactive mode)
	startupInfo := agentLoop.GetStartupInfo()
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      startupInfo["tools"].(map[string]any)["count"],
			"skills_total":     startupInfo["skills"].(map[string]any)["total"],
			"skills_available": startupInfo["skills"].(map[string]any)["available"],
		})

	if message != "" {
		ctx := context.Background()
		response, err := streamWithSpinner(ctx, agentLoop, message, sessionKey)
		if err != nil {
			return fmt.Errorf("error processing message: %w", err)
		}
		// Only print if not already streamed (non-streaming fallback)
		if response != "" {
			fmt.Printf("\n%s %s\n", internal.Logo, response)
		}
		return nil
	}

	fmt.Printf("%s Interactive mode (Ctrl+C to exit)\n\n", internal.Logo)
	interactiveMode(agentLoop, sessionKey)

	return nil
}

// streamWithSpinner runs the agent with streaming output.
// Shows a spinner until the first token arrives, then streams tokens inline.
// Returns the full response only if streaming was NOT used (for the caller to print).
func streamWithSpinner(ctx context.Context, agentLoop *agent.AgentLoop, input, sessionKey string) (string, error) {
	var firstChunk atomic.Bool
	var printed atomic.Int64 // bytes already written to stdout

	// Start spinner in background; stops on first chunk or completion
	spinnerDone := make(chan struct{})
	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			select {
			case <-spinnerDone:
				return
			case <-time.After(80 * time.Millisecond):
				if firstChunk.Load() {
					return
				}
				fmt.Printf("\r%s %s thinking... ", internal.Logo, frames[i%len(frames)])
				i++
			}
		}
	}()

	prefix := fmt.Sprintf("\n%s ", internal.Logo)
	prefixPrinted := false

	onChunk := func(accumulated string) {
		// Strip completed <details>...</details> blocks
		clean := utils.StripHTMLArtifacts(accumulated)
		// Hold back any partial <details> tag that hasn't been closed yet
		if idx := strings.Index(strings.ToLower(clean), "<details"); idx != -1 {
			clean = strings.TrimRight(clean[:idx], " \t\n")
		}
		if clean == "" {
			return
		}
		if !firstChunk.Load() {
			firstChunk.Store(true)
			close(spinnerDone)
			fmt.Print("\r\033[K")
			fmt.Print(prefix)
			prefixPrinted = true
		}
		// Print only the new portion since last chunk
		prev := int(printed.Load())
		if len(clean) > prev {
			fmt.Print(clean[prev:])
			printed.Store(int64(len(clean)))
		}
	}

	response, err := agentLoop.ProcessDirectStream(ctx, input, sessionKey, onChunk)

	// Ensure spinner is stopped
	select {
	case <-spinnerDone:
	default:
		close(spinnerDone)
		fmt.Print("\r\033[K")
	}

	if err != nil {
		return "", err
	}

	if prefixPrinted {
		fmt.Println()
		return "", nil // already streamed
	}
	return response, nil
}

func interactiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	prompt := fmt.Sprintf("%s You: ", internal.Logo)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     filepath.Join(os.TempDir(), ".picoclaw_history"),
		HistoryLimit:    100,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		fmt.Println("Falling back to simple input mode...")
		simpleInteractiveMode(agentLoop, sessionKey)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		ctx := context.Background()
		response, err := streamWithSpinner(ctx, agentLoop, input, sessionKey)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if response != "" {
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)
		} else {
			fmt.Println()
		}
	}
}

func simpleInteractiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(fmt.Sprintf("%s You: ", internal.Logo))
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		ctx := context.Background()
		response, err := streamWithSpinner(ctx, agentLoop, input, sessionKey)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if response != "" {
			fmt.Printf("\n%s %s\n\n", internal.Logo, response)
		} else {
			fmt.Println()
		}
	}
}
