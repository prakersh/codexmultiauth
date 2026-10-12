package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestLimitsAuthFailureIsActionableInTerminalPlainAndJSON(t *testing.T) {
	results := limitsLayoutFixture()
	results[0].Usage.Confidence = domain.UsageConfidenceBestEffort
	results[0].Usage.Quotas = nil
	results[0].Problem = &app.UsageProblem{Code: "refresh_token_expired", StatusCode: 401, RequiresLogin: true,
		Message: "refresh token request failed: status 401 (refresh_token_expired); sign in again with cma login."}
	for _, width := range []int{0, 20, 76, 160, 240} {
		var out bytes.Buffer
		command := &cobra.Command{}
		command.SetOut(&out)
		printLimitsTableAtWidth(command, results, true, width)
		output := out.String()
		require.Contains(t, output, "auth")
		if width == 0 {
			row := findLine(strings.Split(output, "\n"), results[0].Account.DisplayName)
			require.Contains(t, row, "refresh_token_expired")
			require.Contains(t, row, "sign in")
		} else {
			for _, line := range strings.Split(output, "\n") {
				require.LessOrEqual(t, ansi.StringWidth(line), width)
			}
		}
	}
	var out bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&out)
	require.NoError(t, printLimitsJSON(command, results))
	require.Contains(t, out.String(), `"problem":{"code":"refresh_token_expired"`)
	require.Contains(t, out.String(), `"requires_login":true`)
}

func TestLimitsDistinguishesLiveRenewalWarningFromUnavailableUsage(t *testing.T) {
	result := limitsLayoutFixture()[0]
	result.Problem = &app.UsageProblem{RequiresLogin: true, StatusCode: 401}
	require.Equal(t, "live!", confidenceCell(result, "", "", "", "").text)
	result.Usage.Confidence = domain.UsageConfidenceUnknown
	require.Equal(t, "auth", confidenceCell(result, "", "", "", "").text)
	result.Problem = &app.UsageProblem{StatusCode: 403}
	require.Equal(t, "denied", confidenceCell(result, "", "", "", "").text)
	result.Problem = &app.UsageProblem{}
	require.Equal(t, "error", confidenceCell(result, "", "", "", "").text)
}
