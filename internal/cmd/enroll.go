package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	pkgconfig "github.com/bethrou/bethrou/pkg/config"
	"github.com/bethrou/bethrou/pkg/enroll"
	"github.com/bethrou/bethrou/pkg/identity"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	enrollAPIURL   string
	enrollToken    string
	enrollIdentity string
)

// enrollTokenEnvVar lets scripted/CI callers pass the token without putting
// it on the command line, where it would land in shell history and be
// visible to any local user via ps/proc.
const enrollTokenEnvVar = "BETHROU_ENROLL_TOKEN"

// readEnrollToken resolves the enrollment token from, in order: --token
// (kept for compatibility, but discouraged since it's visible via ps and
// shell history), the BETHROU_ENROLL_TOKEN env var, or an interactive
// prompt (with echo suppressed when stdin is a terminal).
func readEnrollToken(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	if fromEnv := os.Getenv(enrollTokenEnvVar); fromEnv != "" {
		return fromEnv, nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("Enrollment token: ")
		tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read enrollment token: %w", err)
		}
		return strings.TrimSpace(string(tokenBytes)), nil
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read enrollment token from stdin: %w", err)
	}
	return strings.TrimSpace(line), nil
}

var enrollCmd = &cobra.Command{
	Use:   "enroll",
	Short: "Redeem a one-time enrollment token issued by the Bethrou control plane",
	RunE: func(cmd *cobra.Command, args []string) error {
		if enrollAPIURL == "" {
			return fmt.Errorf("--api-url is required")
		}

		token, err := readEnrollToken(enrollToken)
		if err != nil {
			return err
		}
		if token == "" {
			return fmt.Errorf("enrollment token is required (--token, %s, or stdin)", enrollTokenEnvVar)
		}
		enrollToken = token

		priv, err := identity.NewManager(enrollIdentity).LoadOrGenerate()
		if err != nil {
			return fmt.Errorf("failed to load or generate identity key: %w", err)
		}

		result, err := enroll.Enroll(context.Background(), enrollAPIURL, enrollToken, priv)
		if err != nil {
			return err
		}

		fmt.Printf("Enrolled as %s peer with id %s\n", result.Role, result.ID)
		fmt.Printf("Nothing else was saved to disk except the identity key at %s.\n", enrollIdentity)
		fmt.Printf("Run 'client connect --identity-key %s --api-url %s --target-node <node-peer-id>' to start.\n", enrollIdentity, enrollAPIURL)

		return nil
	},
}

// registerEnrollFlags registers the enroll subcommand's flags. Called from
// Execute (client/cmd/root.go), after LoadDotEnv, for the same reason
// registerConnectFlags is.
func registerEnrollFlags() {
	enrollCmd.Flags().StringVar(&enrollAPIURL, "api-url", pkgconfig.EnvOr("BETHROU_CLIENT_API_URL", ""), "Control-plane API base URL, e.g. https://saas.example.com (env: BETHROU_CLIENT_API_URL)")
	enrollCmd.Flags().StringVar(&enrollToken, "token", "", "Enrollment token from the web dashboard (discouraged: visible in shell history/ps; prefer the "+enrollTokenEnvVar+" env var or the interactive prompt)")
	enrollCmd.Flags().StringVar(&enrollIdentity, "identity-key", pkgconfig.EnvOr("BETHROU_CLIENT_IDENTITY_KEY", "client.key"), "Path to the persistent client identity key (env: BETHROU_CLIENT_IDENTITY_KEY)")

	rootCmd.AddCommand(enrollCmd)
}
