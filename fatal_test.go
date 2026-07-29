package slog

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const fatalHelperEnv = "SLOG_FATAL_HELPER"

func TestFatalAndFatalfExitContract(t *testing.T) {
	if helper := os.Getenv(fatalHelperEnv); helper != "" {
		runFatalHelper(helper)
		return
	}

	tests := []struct {
		name    string
		helper  string
		message string
	}{
		{name: "package Fatal", helper: "package-fatal", message: "package fatal"},
		{name: "package Fatalf", helper: "package-fatalf", message: "package formatted fatal"},
		{name: "logger Fatal", helper: "logger-fatal", message: "logger fatal"},
		{name: "logger Fatalf", helper: "logger-fatalf", message: "logger formatted fatal"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestFatalAndFatalfExitContract$")
			command.Env = append(os.Environ(), fatalHelperEnv+"="+test.helper)
			output, err := command.CombinedOutput()

			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("exit error = %v, want status 1; output=%q", err, output)
			}
			if !strings.Contains(string(output), test.message) {
				t.Fatalf("output = %q, want fatal message %q", output, test.message)
			}
			if strings.Contains(string(output), "deferred marker") {
				t.Fatalf("defer ran after Fatal: output=%q", output)
			}
		})
	}
}

func runFatalHelper(helper string) {
	defer fmt.Fprintln(os.Stdout, "deferred marker")

	config := DefaultConfig()
	config.NoColor = true
	config.SetEnableText(true)
	config.SetEnableJSON(false)
	switch helper {
	case "package-fatal":
		SetLevelInfo()
		EnableTextLogger()
		DisableJSONLogger()
		ResetGlobalLogger(os.Stdout, true, false)
		Fatal("package fatal")
	case "package-fatalf":
		SetLevelInfo()
		EnableTextLogger()
		DisableJSONLogger()
		ResetGlobalLogger(os.Stdout, true, false)
		Fatalf("package %s fatal", "formatted")
	case "logger-fatal":
		NewLoggerWithConfig(os.Stdout, config).Fatal("logger fatal")
	case "logger-fatalf":
		NewLoggerWithConfig(os.Stdout, config).Fatalf("logger %s fatal", "formatted")
	default:
		fmt.Fprintf(os.Stderr, "unknown fatal helper %q\n", helper)
		os.Exit(2)
	}
}
