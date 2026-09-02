package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

const maximumActionOutput = 32 * 1024

func main() {
	id := envOr("HOMINAL_ORGAN_ID", "system")
	if len(os.Args) < 2 {
		fail("usage: hominal-system describe | health | observe | perform '<action-json>'")
	}
	switch os.Args[1] {
	case "describe":
		write(organ.Description{
			Schema: organ.DescriptionSchema, ID: id, Name: "Ubuntu system", Command: "system",
			Capabilities: []string{"observe", "perform", "cancel", "body_state", "filesystem", "process", "network", "software"},
			Operations:   []string{"exec"},
			Guidance:     "system 提供 Ubuntu 主机、存储、网络、桌面与进程事实。行动时从 operations 选择 exec，并在 input 中写入以 root 身份执行的 bash 源码。",
		})
	case "health":
		write(organ.Health{Schema: organ.HealthSchema, ID: id, Status: "ready", Accepting: true})
	case "observe":
		write(observe(id))
	case "perform":
		if len(os.Args) != 3 {
			fail("perform requires one action envelope")
		}
		write(perform(id, os.Args[2]))
	default:
		fail("unsupported system organ operation")
	}
}

func observe(id string) organ.Observation {
	facts := map[string]any{
		"uptime_seconds":      readUptime(),
		"root_free_bytes":     freeBytes("/"),
		"agent_free_bytes":    freeBytes("/agent"),
		"network_available":   networkAvailable(os.Getenv("HOMINAL_NETWORK_PROBE_URL")),
		"desktop_available":   commandSucceeds(2*time.Second, "systemctl", "is-active", "--quiet", "lightdm"),
		"wechat_running":      commandSucceeds(2*time.Second, "pgrep", "-f", "(^|/)wechat( |$)"),
		"clash_verge_running": commandSucceeds(2*time.Second, "systemctl", "is-active", "--quiet", "clash-verge-service.service"),
	}
	encoded := make(map[string]json.RawMessage, len(facts))
	for key, value := range facts {
		data, _ := json.Marshal(value)
		encoded[key] = data
	}
	return organ.Observation{
		Schema: organ.ObservationSchema, OrganID: id, SurfaceID: "ubuntu.body",
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Facts: encoded,
	}
}

func perform(id, encoded string) organ.ActionResult {
	var request organ.ActionRequest
	if err := json.Unmarshal([]byte(encoded), &request); err != nil || request.Schema != organ.ActionSchema ||
		strings.TrimSpace(request.ActionID) == "" || strings.TrimSpace(request.Operation) == "" {
		fail("invalid organ action envelope")
	}
	result := organ.ActionResult{
		Schema: organ.ActionResultSchema, OrganID: id, ActionID: request.ActionID,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if request.Operation != "exec" {
		result.Status = "failed"
		result.Summary = "System Organ 不支持该操作。"
		result.Output = compactJSON(map[string]any{"operation": request.Operation, "error": "unsupported operation"})
		return result
	}
	if strings.TrimSpace(request.Input) == "" {
		result.Status = "failed"
		result.Summary = "System Organ 收到的 bash 源码为空。"
		result.Output = compactJSON(map[string]any{"error": "empty input"})
		return result
	}
	timeout := time.Duration(request.TimeoutMilliseconds) * time.Millisecond
	if timeout <= 0 || timeout > 2*time.Minute {
		timeout = 30 * time.Second
	}
	status, output := execute(request.Input, timeout)
	result.Status = status
	result.Output = output
	switch status {
	case "completed":
		result.Summary = "System Organ 已完成操作并取得退出状态与输出。"
	case "failed":
		result.Summary = "System Organ 已完成尝试，现实结果表明操作失败。"
	default:
		result.Summary = "System Organ 的操作被中断，现实是否已经部分改变尚不确定。"
	}
	return result
}

func execute(source string, timeout time.Duration) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-lc", source)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return killGroup(command) }
	command.WaitDelay = 2 * time.Second
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &limitedWriter{writer: &stdout, remaining: maximumActionOutput}
	command.Stderr = &limitedWriter{writer: &stderr, remaining: maximumActionOutput}

	signals := make(chan os.Signal, 1)
	var externallyCancelled atomic.Bool
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			externallyCancelled.Store(true)
			_ = killGroup(command)
		case <-done:
		}
	}()
	err := command.Run()
	close(done)
	signal.Stop(signals)

	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	status := "completed"
	if timedOut || externallyCancelled.Load() {
		status = "unknown"
	} else if err != nil {
		status = "failed"
	}
	return status, compactJSON(map[string]any{
		"operation": "exec", "exit_code": exitCode, "stdout": stdout.String(), "stderr": stderr.String(),
		"timed_out": timedOut, "cancelled": externallyCancelled.Load(),
	})
}

func killGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	original := len(data)
	if w.remaining <= 0 {
		return original, nil
	}
	if len(data) > w.remaining {
		data = data[:w.remaining]
	}
	_, err := w.writer.Write(data)
	w.remaining -= len(data)
	return original, err
}

func readUptime() int64 {
	file, err := os.Open("/proc/uptime")
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func freeBytes(path string) uint64 {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0
	}
	return stats.Bavail * uint64(stats.Bsize)
}

func networkAvailable(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, parsed.String(), nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}

func commandSucceeds(timeout time.Duration, name string, arguments ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, arguments...).Run() == nil
}

func compactJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func write(value any) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
