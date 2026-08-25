package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	hominal "hominal.cc/hominal/body/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	instanceRoot := os.Getenv("HOMINAL_INSTANCE_ROOT")
	instanceID := os.Getenv("HOMINAL_INSTANCE_ID")
	if instanceRoot == "" || instanceID == "" {
		return errors.New("HOMINAL_INSTANCE_ROOT and HOMINAL_INSTANCE_ID are required")
	}
	configPath := os.Getenv("HOMINAL_RUNTIME_CONFIG")
	if configPath == "" {
		configPath = "/etc/hominal/runtime.json"
	}
	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("open runtime config: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config hominal.Config
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("decode runtime config: %w", err)
	}
	if config.Model.APIKey == "" || config.Model.BaseURL == "" || config.Model.Name == "" {
		return errors.New("runtime model configuration is incomplete")
	}
	runtime, err := hominal.New(instanceRoot, instanceID, config, hominal.NewModelClient())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runtime.Run(ctx)
}
