package main

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
)

func TestInitConfig_Defaults(t *testing.T) {
	resetCommandTest(t)

	root := newRootCommand()
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("find serve command: %v", err)
	}
	if err := initConfig(serveCmd); err != nil {
		t.Fatalf("initConfig() error = %v", err)
	}

	if got := viper.GetString("server.host"); got != "localhost" {
		t.Fatalf("server.host = %q; want localhost", got)
	}
	if got := viper.GetString("server.port"); got != "8080" {
		t.Fatalf("server.port = %q; want 8080", got)
	}
	if got := viper.GetBool("debug.pprof.enabled"); got {
		t.Fatalf("debug.pprof.enabled = true; want false")
	}
}

func TestInitConfig_EnvAndFlagPrecedence(t *testing.T) {
	resetCommandTest(t)
	t.Setenv("PLETKA_SERVER_PORT", "7777")
	t.Setenv("PLETKA_LOG_FORMAT", "json")

	root := newRootCommand()
	if err := root.PersistentFlags().Set("log-format", "text"); err != nil {
		t.Fatalf("set log-format flag: %v", err)
	}
	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("find serve command: %v", err)
	}
	if err := serveCmd.Flags().Set("port", "9090"); err != nil {
		t.Fatalf("set port flag: %v", err)
	}

	if err := initConfig(serveCmd); err != nil {
		t.Fatalf("initConfig() error = %v", err)
	}

	if got := viper.GetString("server.port"); got != "9090" {
		t.Fatalf("server.port = %q; want flag value 9090", got)
	}
	if got := viper.GetString("log.format"); got != "text" {
		t.Fatalf("log.format = %q; want flag value text", got)
	}
}

func TestRootCommand_VersionDoesNotNeedConfig(t *testing.T) {
	resetCommandTest(t)

	root := newRootCommand()
	out := bytes.NewBuffer(nil)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command error = %v", err)
	}
	if got := out.String(); got == "" {
		t.Fatalf("version command wrote no output")
	}
}

func resetCommandTest(t *testing.T) {
	t.Helper()
	viper.Reset()
	cfgFile = ""
	logger = nil
}
