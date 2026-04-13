package codexcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Client struct {
	Binary string
}

func NewClient(binary string) *Client {
	if strings.TrimSpace(binary) == "" {
		binary = "codex"
	}
	return &Client{Binary: binary}
}

func (c *Client) Login(ctx context.Context, deviceAuth bool, withAPIKey bool) error {
	args := []string{"login"}
	if deviceAuth {
		args = append(args, "--device-auth")
	}
	if withAPIKey {
		args = append(args, "--with-api-key")
	}
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run codex login: %w", err)
	}
	return nil
}

func (c *Client) Status(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, "login", "status")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run codex login status: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}
