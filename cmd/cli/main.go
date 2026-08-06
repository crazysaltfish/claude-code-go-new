package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-code-go/internal/cli"
)

var (
	// Version information
	version = "2.1.88"

	// CLI flags
	debugFlag      bool
	verboseFlag    bool
	printModeFlag  bool
	modelFlag      string
	providerFlag   string
	baseURLFlag    string
	permissionFlag string
	maxTokensFlag  int
	maxTurnsFlag   int
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "claude [flags] [prompt]",
	Short: "Claude Code - AI-powered coding assistant",
	Long: `Claude Code is an AI-powered coding assistant that helps you write,
edit, and understand code. It can execute commands, read and write files,
and help with various development tasks.

Use Claude right from your terminal to:
- Understand and navigate codebases
- Write and edit code
- Execute terminal commands
- Debug and fix issues
- And much more!`,
	Version: version,
	Run:     runMain,
	Args:    cobra.ArbitraryArgs,
}

func init() {
	rootCmd.Flags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug mode")
	rootCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose output")
	rootCmd.Flags().BoolVarP(&printModeFlag, "print", "p", false, "Print mode (non-interactive)")
	rootCmd.Flags().StringVarP(&modelFlag, "model", "m", "", "Model to use")
	rootCmd.Flags().StringVar(&providerFlag, "provider", "", "API provider: anthropic or openai")
	rootCmd.Flags().StringVar(&baseURLFlag, "base-url", "", "Override the provider API base URL")
	rootCmd.Flags().StringVar(&permissionFlag, "permission-mode", "", "Permission mode (default, acceptEdits, bypassPermissions)")
	rootCmd.Flags().IntVar(&maxTokensFlag, "max-tokens", 4096, "Maximum output tokens per API response")
	rootCmd.Flags().IntVar(&maxTurnsFlag, "max-turns", 100, "Maximum number of conversation turns")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(doctorCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Claude Code v%s\n", version)
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Configuration management")
		// TODO: Implement config management
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server management",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("MCP management")
		// TODO: Implement MCP management
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run diagnostics and check system health",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running diagnostics...")
		// TODO: Implement doctor functionality
	},
}

func runMain(cmd *cobra.Command, args []string) {
	// Create app config
	config := &cli.Config{
		Debug:          debugFlag,
		Verbose:        verboseFlag,
		PrintMode:      printModeFlag,
		Model:          modelFlag,
		Provider:       providerFlag,
		BaseURL:        baseURLFlag,
		PermissionMode: permissionFlag,
		MaxTokens:      maxTokensFlag,
		MaxTurns:       maxTurnsFlag,
	}

	// Create and initialize app
	app := cli.NewApp(config, version)
	if err := app.Initialize(); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize:", err)
		os.Exit(1)
	}
	defer app.Shutdown()

	// Get initial prompt from args
	var initialPrompt string
	if len(args) > 0 {
		initialPrompt = joinArgs(args)
	}

	// Run the app
	if err := app.Run(initialPrompt); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// joinArgs joins command line arguments into a single string.
func joinArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
