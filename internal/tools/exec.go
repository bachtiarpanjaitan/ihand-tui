package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/bachtiarpanjaitan/ihandai-go/pkg/core"
)

// ---------------------------------------------------------------------------
// ExecTool — menjalankan perintah shell di direktori project
// ---------------------------------------------------------------------------

type ExecTool struct {
	allowedDir string
}

func NewExecTool(allowedDir string) *ExecTool {
	return &ExecTool{allowedDir: allowedDir}
}

func (t *ExecTool) Name() string { return "exec" }

func (t *ExecTool) Description() string {
	return "Menjalankan perintah shell/terminal di direktori project. " +
		"Gunakan untuk: compile/run kode (go build, g++, python, dll), " +
		"install dependencies, test, git, dan perintah shell lainnya. " +
		"Timeout 60 detik. " +
		"Contoh: exec({\"command\": \"go build ./...\"}) " +
		"Contoh: exec({\"command\": \"g++ -o main main.cpp && ./main\"})" +
		"PENTING: perintah dijalankan di direktori project (allowedDir)."
}

func (t *ExecTool) InputSchema() *core.JSONSchema {
	return &core.JSONSchema{
		Type: "object",
		Properties: map[string]*core.JSONSchemaProp{
			"command": {Type: "string", Description: "Perintah shell yang akan dijalankan"},
		},
		Required: []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error": "invalid input: %s"}`, err.Error())), nil
	}

	if strings.TrimSpace(params.Command) == "" {
		return json.RawMessage(`{"error": "command tidak boleh kosong"}`), nil
	}

	// Execute dengan timeout 60 detik
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command)
	cmd.Dir = t.allowedDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			exitCode = -1
			return json.RawMessage(fmt.Sprintf(`{
				"exit_code": -1,
				"stdout": %s,
				"stderr": "TIMEOUT: perintah tidak selesai dalam 60 detik",
				"error": "timeout"
			}`, jsonString(stdout.String()))), nil
		} else {
			exitCode = -2
		}
	}

	outStr := stdout.String()
	errStr := stderr.String()

	// Batasi output maksimal 10000 karakter
	maxOut := 10000
	if len(outStr) > maxOut {
		outStr = outStr[:maxOut] + "\n... (output truncated)"
	}
	if len(errStr) > maxOut {
		errStr = errStr[:maxOut] + "\n... (stderr truncated)"
	}

	result := map[string]any{
		"exit_code": exitCode,
		"stdout":    outStr,
		"stderr":    errStr,
	}
	if exitCode != 0 {
		result["error"] = fmt.Sprintf("exit code %d", exitCode)
	}

	data, _ := json.Marshal(result)
	return json.RawMessage(data), nil
}

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
