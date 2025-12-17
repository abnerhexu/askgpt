package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultAPIURL    = "https://api.openai.com/v1/chat/completions"
	defaultModelName = "gpt-4o-mini"

	appDirName      = ".askgpt"
	configFileName  = "config.yaml"
	configFilePerm  = 0o600
	configDirPerm   = 0o700
	httpTimeout     = 5 * time.Minute
	defaultMaxToken = 1024
)

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// For streaming response chunk
type ChatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type AskGPTConfig struct {
	URL   string
	Model string
	Key   string
}

// Unmarshal YAML supporting both shapes:
// 1) requested list-of-maps:
// askgpt:
//   - url: ...
//   - model: ...
//   - key: ...
//
// 2) common mapping:
// askgpt:
//
//	url: ...
//	model: ...
//	key: ...
func (c *AskGPTConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		var tmp struct {
			URL   string `yaml:"url"`
			Model string `yaml:"model"`
			Key   string `yaml:"key"`
		}
		if err := value.Decode(&tmp); err != nil {
			return err
		}
		c.URL, c.Model, c.Key = tmp.URL, tmp.Model, tmp.Key
		return nil
	case yaml.SequenceNode:
		for _, item := range value.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			// mapping node content is [k1,v1,k2,v2,...]
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := item.Content[i]
				v := item.Content[i+1]
				if k.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode {
					continue
				}
				switch strings.TrimSpace(k.Value) {
				case "url":
					c.URL = strings.TrimSpace(v.Value)
				case "model":
					c.Model = strings.TrimSpace(v.Value)
				case "key":
					c.Key = strings.TrimSpace(v.Value)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("askgpt config must be mapping or sequence, got kind=%d", value.Kind)
	}
}

// Marshal YAML in the exact format the user requested (sequence of maps).
func (c AskGPTConfig) MarshalYAML() (any, error) {
	type kv map[string]string
	return []kv{
		{"url": c.URL},
		{"model": c.Model},
		{"key": c.Key},
	}, nil
}

type ConfigFile struct {
	AskGPT AskGPTConfig `yaml:"askgpt"`
}

func getPrompt(task, input string) string {
	switch task {
	case "chat":
		return input
	case "translate-en":
		return "Translate the following text into English:\n\n" + input
	case "translate-zh":
		return "将下列内容翻译为中文：\n\n" + input
	case "summarize":
		return "总结下面的内容：\n\n" + input
	case "explain":
		return "解释下面的内容：\n\n" + input
	default:
		return input
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	return filepath.Join(home, appDirName, configFileName), nil
}

func ensureConfigFileExists() (path string, created bool, err error) {
	path, err = configPath()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), configDirPerm); err != nil {
		return "", false, fmt.Errorf("cannot create dir %s: %w", filepath.Dir(path), err)
	}
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	template := ConfigFile{
		AskGPT: AskGPTConfig{
			URL:   defaultAPIURL,
			Model: defaultModelName,
			Key:   "",
		},
	}
	if err := writeConfigFile(path, template); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func loadConfigFile(path string) (ConfigFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	var cfg ConfigFile
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return ConfigFile{}, fmt.Errorf("cannot parse yaml %s: %w", path, err)
	}
	return cfg, nil
}

func writeConfigFile(path string, cfg ConfigFile) error {
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("cannot marshal yaml: %w", err)
	}

	// Add a small header comment; YAML remains valid.
	content := strings.Join([]string{
		"# askgpt config",
		"# You can edit this file directly, or use: askgpt set-url | set-model | set-key",
		string(out),
	}, "\n")

	if err := os.WriteFile(path, []byte(content), configFilePerm); err != nil {
		return fmt.Errorf("cannot write config %s: %w", path, err)
	}
	return nil
}

func validateRuntimeConfig(cfg ConfigFile) error {
	if strings.TrimSpace(cfg.AskGPT.URL) == "" {
		return errors.New("missing askgpt.url in config.yaml")
	}
	if strings.TrimSpace(cfg.AskGPT.Model) == "" {
		return errors.New("missing askgpt.model in config.yaml")
	}
	if strings.TrimSpace(cfg.AskGPT.Key) == "" {
		return errors.New("missing askgpt.key in config.yaml")
	}
	return nil
}

func readSingleLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// readInput reads user input in a more "Enter feels done" way:
// - Single-line input: just press Enter.
// - Multi-line input: end a line with a backslash "\" to continue, or use ":paste" mode.
// - Commands:
//   - ":paste" -> enter paste mode, finish with a single line ":end"
//   - "quit"   -> caller can treat as exit signal
func readInput(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)

	reader := bufio.NewReader(os.Stdin)
	var lines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}

		trimmedRight := strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(trimmedRight)

		if errors.Is(err, io.EOF) {
			if trimmedRight == "" && len(lines) == 0 {
				return "", nil
			}
			if trimmedRight != "" {
				lines = append(lines, trimmedRight)
			}
			break
		}

		if len(lines) == 0 && trimmed == ":paste" {
			fmt.Fprint(os.Stderr, "Paste mode: end with a single line \":end\"\n")
			for {
				pl, perr := reader.ReadString('\n')
				if perr != nil && !errors.Is(perr, io.EOF) {
					return "", perr
				}
				pr := strings.TrimRight(pl, "\r\n")
				pt := strings.TrimSpace(pr)

				if pt == ":end" {
					return strings.Join(lines, "\n"), nil
				}

				if errors.Is(perr, io.EOF) {
					if pr != "" {
						lines = append(lines, pr)
					}
					return strings.Join(lines, "\n"), nil
				}

				lines = append(lines, pr)
			}
		}

		if strings.HasSuffix(trimmedRight, `\`) {
			lines = append(lines, strings.TrimSuffix(trimmedRight, `\`))
			continue
		}

		lines = append(lines, trimmedRight)
		break
	}

	return strings.Join(lines, "\n"), nil
}

func doStreamingChat(client *http.Client, cfg AskGPTConfig, messages []Message) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   defaultMaxToken,
		Stream:      true,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", cfg.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.Key)

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("api error (%d): %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	var fullResponse strings.Builder

	fmt.Print("Assistant: ")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fullResponse.String(), fmt.Errorf("stream read error: %w", err)
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var chunk ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				content := chunk.Choices[0].Delta.Content
				fmt.Print(content)
				fullResponse.WriteString(content)
			}
		}
	}
	fmt.Println()
	return fullResponse.String(), nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "     ___           _______. __  ___   _______ .______   .___________.")
	fmt.Fprintln(os.Stderr, "    /   \\         /       ||  |/  /  /  _____||   _  \\  |           |")
	fmt.Fprintln(os.Stderr, "   /  ^  \\       |   (----`|  '  /  |  |  __  |  |_)  | `---|  |----`")
	fmt.Fprintln(os.Stderr, "  /  /_\\  \\       \\   \\    |    <   |  | |_ | |   ___/      |  |     ")
	fmt.Fprintln(os.Stderr, " /  _____  \\  .----)   |   |  .  \\  |  |__| | |  |          |  |     ")
	fmt.Fprintln(os.Stderr, "/__/     \\__\\ |_______/    |__|\\__\\  \\______| | _|          |__|     ")
	fmt.Fprintln(os.Stderr)
	base := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage: %s [command] [arguments]\n\n", base)

	fmt.Fprintln(os.Stderr, "Configuration:")
	fmt.Fprintf(os.Stderr, "  %-20s Show current configuration\n", "show-config")
	fmt.Fprintf(os.Stderr, "  %-20s Set OpenAI API URL\n", "set-url <value>")
	fmt.Fprintf(os.Stderr, "  %-20s Set OpenAI Model (e.g., gpt-4o)\n", "set-model <value>")
	fmt.Fprintf(os.Stderr, "  %-20s Set OpenAI API Key\n", "set-key <value>")
	fmt.Fprintf(os.Stderr, "  %-20s Generate completion script\n", "completion <shell>")
	fmt.Fprintln(os.Stderr)

	fmt.Fprintln(os.Stderr, "Tasks:")
	fmt.Fprintf(os.Stderr, "  %-20s Run a specific task\n", "<task>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  Available tasks:")
	fmt.Fprintf(os.Stderr, "    %-18s Start a chat session without prompt template\n", "chat")
	fmt.Fprintf(os.Stderr, "    %-18s Translate text to English\n", "translate-en")
	fmt.Fprintf(os.Stderr, "    %-18s Translate text to Chinese\n", "translate-zh")
	fmt.Fprintf(os.Stderr, "    %-18s Summarize content\n", "summarize")
	fmt.Fprintf(os.Stderr, "    %-18s Explain content\n", "explain")
	fmt.Fprintf(os.Stderr, "    %-18s Any other string is sent as a direct prompt\n", "(direct prompt)")
	fmt.Fprintln(os.Stderr)

}

func runShowConfig() int {
	path, created, err := ensureConfigFileExists()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if created {
		fmt.Fprintf(os.Stderr, "Created config template at %s\n", path)
		fmt.Fprintln(os.Stderr, "Please fill url/model/key (edit the file or run set-url/set-model/set-key), then rerun.")
		return 1
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot marshal config: %v\n", err)
		return 1
	}

	// Print to stdout for piping
	fmt.Print(string(out))
	return 0
}

func runSetCommand(cmd string, maybeValue string) int {
	path, _, err := ensureConfigFileExists()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		// If file exists but is malformed, don't overwrite silently.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	value := strings.TrimSpace(maybeValue)
	if value == "" {
		switch cmd {
		case "set-url":
			value, err = readSingleLine("Enter api url: ")
		case "set-model":
			value, err = readSingleLine("Enter model: ")
		case "set-key":
			value, err = readSingleLine("Enter api key: ")
		default:
			fmt.Fprintln(os.Stderr, "Unknown set command.")
			return 1
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading value: %v\n", err)
			return 1
		}
		value = strings.TrimSpace(value)
	}

	if value == "" {
		fmt.Fprintln(os.Stderr, "Error: empty value not allowed")
		return 1
	}

	switch cmd {
	case "set-url":
		cfg.AskGPT.URL = value
	case "set-model":
		cfg.AskGPT.Model = value
	case "set-key":
		cfg.AskGPT.Key = value
	default:
		fmt.Fprintln(os.Stderr, "Unknown set command.")
		return 1
	}

	if err := writeConfigFile(path, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Updated %s successfully.\n", path)
	return 0
}

const bashCompletion = `_askgpt_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="show-config set-url set-model set-key chat translate-en translate-zh summarize explain completion"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi
}
complete -F _askgpt_completion askgpt
`

const zshCompletion = `#compdef askgpt

_askgpt() {
    local -a commands
    commands=(
        'show-config:Show current configuration'
        'set-url:Set OpenAI API URL'
        'set-model:Set OpenAI Model'
        'set-key:Set OpenAI API Key'
        'chat:Start a chat session without prompt template'
        'translate-en:Translate text to English'
        'translate-zh:Translate text to Chinese'
        'summarize:Summarize content'
        'explain:Explain content'
        'completion:Generate completion script'
    )
    _describe -t commands 'commands' commands
}

_askgpt
`

const fishCompletion = `set -l commands show-config set-url set-model set-key chat translate-en translate-zh summarize explain completion
complete -c askgpt -f
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "show-config" -d "Show current configuration"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "set-url" -d "Set OpenAI API URL"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "set-model" -d "Set OpenAI Model"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "set-key" -d "Set OpenAI API Key"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "chat" -d "Start a chat session without prompt template"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "translate-en" -d "Translate text to English"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "translate-zh" -d "Translate text to Chinese"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "summarize" -d "Summarize content"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "explain" -d "Explain content"
complete -c askgpt -n "not __fish_seen_subcommand_from $commands" -a "completion" -d "Generate completion script"
`

func runCompletion(shell string) int {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell: %s. Supported: bash, zsh, fish\n", shell)
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "show-config":
		os.Exit(runShowConfig())
	case "completion":
		shell := ""
		if len(os.Args) >= 3 {
			shell = os.Args[2]
		}
		os.Exit(runCompletion(shell))
	case "-h", "help", "--help":
		usage()
		os.Exit(0)
	case "set-url", "set-model", "set-key":
		val := ""
		if len(os.Args) >= 3 {
			val = strings.Join(os.Args[2:], " ")
		}
		os.Exit(runSetCommand(cmd, val))
	}

	// Normal task mode
	task := cmd

	path, created, err := ensureConfigFileExists()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if created {
		fmt.Fprintf(os.Stderr, "Created config template at %s\n", path)
		fmt.Fprintln(os.Stderr, "Please fill url/model/key (edit the file or run set-url/set-model/set-key), then rerun.")
		os.Exit(1)
	}

	cfgFile, err := loadConfigFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := validateRuntimeConfig(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Hint: edit %s or run set-url/set-model/set-key\n", path)
		os.Exit(1)
	}

	client := &http.Client{Timeout: httpTimeout}
	var messages []Message

	fmt.Fprintln(os.Stderr, "Input tips:")
	fmt.Fprintln(os.Stderr, "- Single line: type and press Enter")
	fmt.Fprintln(os.Stderr, "- Multi line: end a line with \\ to continue, or type :paste then finish with :end")
	fmt.Fprintln(os.Stderr, "- Quit: type quit and press Enter")
	fmt.Fprintln(os.Stderr, "")

	userInput, err := readInput("Your message:\n> ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(userInput) == "" {
		fmt.Fprintln(os.Stderr, "No input received.")
		os.Exit(1)
	}
	if strings.TrimSpace(userInput) == "quit" {
		fmt.Fprintln(os.Stderr, "Goodbye!")
		return
	}

	prompt := getPrompt(task, userInput)
	messages = append(messages, Message{Role: "user", Content: prompt})

	for {
		respText, err := doStreamingChat(client, cfgFile.AskGPT, messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		messages = append(messages, Message{Role: "assistant", Content: respText})

		fmt.Fprintln(os.Stderr, "\n---")
		nextInput, err := readInput("Your next message:\n> ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		if strings.TrimSpace(nextInput) == "quit" {
			break
		}
		if strings.TrimSpace(nextInput) == "" {
			continue
		}
		messages = append(messages, Message{Role: "user", Content: nextInput})
	}

	fmt.Fprintln(os.Stderr, "\nGoodbye!")
}
