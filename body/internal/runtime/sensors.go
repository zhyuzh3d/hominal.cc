package runtime

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const diskEventThreshold = 64 * 1024 * 1024

func collectSnapshot(config Config, usedTokens int, slow bool) BodySnapshot {
	current := BodySnapshot{
		ObservedAt:      nowUTC(),
		UptimeSeconds:   readUptime(),
		RootFreeBytes:   freeBytes("/"),
		AgentFreeBytes:  freeBytes("/agent"),
		QuotaUsedTokens: usedTokens,
		QuotaRemaining:  maxInt(config.Quota.LimitTokens-usedTokens, 0),
	}
	if !slow {
		return current
	}
	current.NetworkAvailable = networkAvailable(config.Model.BaseURL)
	current.DesktopAvailable = commandSucceeds(2*time.Second, "systemctl", "is-active", "--quiet", "lightdm")
	_, chromeErr := exec.LookPath("google-chrome")
	current.ChromeAvailable = chromeErr == nil
	_, playwrightErr := exec.LookPath("hominal-playwright-mcp")
	current.PlaywrightReady = playwrightErr == nil
	current.WechatRunning = commandSucceeds(2*time.Second, "pgrep", "-f", "(^|/)wechat( |$)")
	return current
}

func mergeFastSnapshot(previous, fast BodySnapshot) BodySnapshot {
	fast.NetworkAvailable = previous.NetworkAvailable
	fast.DesktopAvailable = previous.DesktopAvailable
	fast.ChromeAvailable = previous.ChromeAvailable
	fast.PlaywrightReady = previous.PlaywrightReady
	fast.WechatRunning = previous.WechatRunning
	return fast
}

func bodyDifferences(previous, current BodySnapshot, initial bool) []string {
	if initial {
		return []string{"initial body snapshot"}
	}
	var differences []string
	if current.UptimeSeconds < previous.UptimeSeconds {
		differences = append(differences, "system uptime restarted")
	}
	if absUint(current.RootFreeBytes, previous.RootFreeBytes) >= diskEventThreshold {
		differences = append(differences, fmt.Sprintf("root free bytes changed from %d to %d", previous.RootFreeBytes, current.RootFreeBytes))
	}
	if absUint(current.AgentFreeBytes, previous.AgentFreeBytes) >= diskEventThreshold {
		differences = append(differences, fmt.Sprintf("agent free bytes changed from %d to %d", previous.AgentFreeBytes, current.AgentFreeBytes))
	}
	previousQuotaBand := quotaResourceBand(previous)
	currentQuotaBand := quotaResourceBand(current)
	if previousQuotaBand != "" && currentQuotaBand != "" && previousQuotaBand != currentQuotaBand {
		differences = append(differences, fmt.Sprintf(
			"model quota resource band changed from %s to %s; %d of %d tokens remain in the rolling window",
			previousQuotaBand,
			currentQuotaBand,
			current.QuotaRemaining,
			current.QuotaUsedTokens+current.QuotaRemaining,
		))
	}
	booleanDifference := func(label string, before, after bool) {
		if before != after {
			differences = append(differences, fmt.Sprintf("%s changed from %t to %t", label, before, after))
		}
	}
	booleanDifference("network availability", previous.NetworkAvailable, current.NetworkAvailable)
	booleanDifference("desktop availability", previous.DesktopAvailable, current.DesktopAvailable)
	booleanDifference("chrome availability", previous.ChromeAvailable, current.ChromeAvailable)
	booleanDifference("playwright availability", previous.PlaywrightReady, current.PlaywrightReady)
	booleanDifference("wechat running", previous.WechatRunning, current.WechatRunning)
	return differences
}

func quotaResourceBand(snapshot BodySnapshot) string {
	total := snapshot.QuotaUsedTokens + snapshot.QuotaRemaining
	if total <= 0 {
		return ""
	}
	percent := 100 * snapshot.QuotaRemaining / total
	switch {
	case percent >= 75:
		return "open"
	case percent >= 50:
		return "comfortable"
	case percent >= 25:
		return "limited"
	case percent >= 10:
		return "scarce"
	default:
		return "critical"
	}
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
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(parsed.Hostname(), port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func commandSucceeds(timeout time.Duration, command string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, command, args...).Run() == nil
}

func absUint(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
