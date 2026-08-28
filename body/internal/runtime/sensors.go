package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const diskEventThreshold = 64 * 1024 * 1024
const perceptualContentLimit = 12 * 1024
const browserPerceptionSurface = "chrome_visible_content"

var (
	browserArticlePattern     = regexp.MustCompile(`article "(.*)" \[ref=`)
	browserNamedObjectPattern = regexp.MustCompile(`(?:heading|link) "([^"]+)"`)
	relativeTimePattern       = regexp.MustCompile(`(?i)\b\d+\s+(?:seconds?|minutes?|hours?|days?)\s+ago\b`)
	engagementPattern         = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?[KMB]?\s+(reply|replies|repost|reposts|like|likes|bookmark|bookmarks|view|views)\b`)
	mediaClockPattern         = regexp.MustCompile(`(?i)\b(?:play\s+)?\d{1,2}:\d{2}\b`)
	mediaSurfacePattern       = regexp.MustCompile(`(?i)\s+(?:embedded video|previous image|<metric>\s+(?:reply|replies))\b.*$`)
)

type browserSemanticScope struct {
	indent int
	role   string
}

type perceptualObservation struct {
	Surface string
	Digest  string
	Content string
	Context []string
	Objects []string
}

func collectSnapshot(config Config, state State, slow bool) BodySnapshot {
	current := BodySnapshot{
		ObservedAt:     nowUTC(),
		UptimeSeconds:  readUptime(),
		RootFreeBytes:  freeBytes("/"),
		AgentFreeBytes: freeBytes("/agent"),
	}
	updateResourceSnapshot(&current, state, config.CognitiveResource, time.Now().UTC())
	if !slow {
		return current
	}
	current.NetworkAvailable = networkAvailable(config.ModelGateway.BaseURL)
	current.DesktopAvailable = commandSucceeds(2*time.Second, "systemctl", "is-active", "--quiet", "lightdm")
	_, chromeErr := exec.LookPath("google-chrome")
	current.ChromeAvailable = chromeErr == nil
	_, playwrightErr := exec.LookPath("hominal-playwright-mcp")
	current.PlaywrightReady = playwrightErr == nil
	current.WechatRunning = commandSucceeds(2*time.Second, "pgrep", "-f", "(^|/)wechat( |$)")
	current.ClashVergeRunning = commandSucceeds(2*time.Second, "systemctl", "is-active", "--quiet", "clash-verge-service.service")
	return current
}

func mergeFastSnapshot(previous, fast BodySnapshot) BodySnapshot {
	fast.NetworkAvailable = previous.NetworkAvailable
	fast.DesktopAvailable = previous.DesktopAvailable
	fast.ChromeAvailable = previous.ChromeAvailable
	fast.PlaywrightReady = previous.PlaywrightReady
	fast.WechatRunning = previous.WechatRunning
	fast.ClashVergeRunning = previous.ClashVergeRunning
	return fast
}

func collectBrowserPerception(instanceRoot string) (perceptualObservation, error) {
	if observation, err := collectXBrowserPerception(instanceRoot); err == nil {
		return observation, nil
	}
	output, err := callBrowserBody(instanceRoot, "browser_snapshot", map[string]any{})
	if err != nil {
		return perceptualObservation{}, err
	}
	text, err := browserSnapshotText(output)
	if err != nil {
		return perceptualObservation{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return perceptualObservation{}, fmt.Errorf("browser snapshot contained no visible text")
	}
	contextLines, objects := browserSemanticObjects(text)
	semantic := browserSemanticSnapshotFromObjects(contextLines, objects)
	if semantic == "" {
		return perceptualObservation{}, fmt.Errorf("browser snapshot contained no stable semantic content")
	}
	digest := sha256.Sum256([]byte(semantic))
	return perceptualObservation{
		Surface: browserPerceptionSurface,
		Digest:  hex.EncodeToString(digest[:]),
		Content: truncate(semantic, perceptualContentLimit),
		Context: contextLines,
		Objects: objects,
	}, nil
}

type browserDOMObject struct {
	Text      string `json:"text"`
	DirectURL string `json:"direct_url"`
}

type browserDOMPerception struct {
	URL     string             `json:"url"`
	Title   string             `json:"title"`
	Objects []browserDOMObject `json:"objects"`
}

// collectXBrowserPerception obtains object identity and its contact route in
// one browser session. A second MCP process is not used because it may attach
// to another tab and silently pair an object with the wrong affordance.
func collectXBrowserPerception(instanceRoot string) (perceptualObservation, error) {
	const code = `async (page) => {
  const url = page.url();
  if (!/^https:\/\/(?:www\.)?x\.com\//i.test(url)) return {url, title: await page.title(), objects: []};
  const objects = await page.locator("article").evaluateAll(nodes => nodes
    .filter(node => node.getClientRects().length > 0)
    .slice(0, 8)
    .map(node => {
      const links = Array.from(node.querySelectorAll('a[href]'));
      const status = links.find(link => /\/status\/\d+(?:[?#].*)?$/.test(link.href));
      const external = links.find(link => {
        try {
          const target = new globalThis.URL(link.href, url);
          return target.protocol === "https:" && !/(^|\.)x\.com$/i.test(target.hostname);
        } catch (_) {
          return false;
        }
      });
      const direct = status || external;
      return {text: (node.innerText || "").slice(0, 2400), direct_url: direct ? direct.href : ""};
    }));
  return {url, title: await page.title(), objects};
}`
	raw, err := callBrowserBody(instanceRoot, "browser_run_code_unsafe", map[string]any{"code": code})
	if err != nil {
		return perceptualObservation{}, err
	}
	var result browserDOMPerception
	if err := browserToolResultJSON(raw, &result); err != nil {
		return perceptualObservation{}, err
	}
	return xBrowserDOMObservation(result)
}

func xBrowserDOMObservation(result browserDOMPerception) (perceptualObservation, error) {
	parsed, err := url.Parse(result.URL)
	if err != nil || strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www.")) != "x.com" {
		return perceptualObservation{}, errors.New("current browser surface is not X")
	}
	contextLines := []string{"Page URL: " + result.URL, "Page Title: " + strings.TrimSpace(result.Title)}
	objects := make([]string, 0, len(result.Objects))
	for _, item := range result.Objects {
		object := normalizeBrowserSemanticText(item.Text)
		if object == "" {
			continue
		}
		if direct := validDirectBrowserURL(item.DirectURL); direct != "" {
			object += " Direct URL: " + direct
		}
		objects = append(objects, object)
	}
	semantic := browserSemanticSnapshotFromObjects(contextLines, objects)
	if len(objects) == 0 || semantic == "" {
		return perceptualObservation{}, errors.New("X DOM contained no visible authored objects")
	}
	digest := sha256.Sum256([]byte(semantic))
	return perceptualObservation{
		Surface: browserPerceptionSurface,
		Digest:  hex.EncodeToString(digest[:]),
		Content: truncate(semantic, perceptualContentLimit),
		Context: contextLines,
		Objects: objects,
	}, nil
}

func browserToolResultJSON(raw []byte, target any) error {
	text, err := browserSnapshotText(raw)
	if err != nil {
		return err
	}
	const resultMarker = "### Result\n"
	start := strings.Index(text, resultMarker)
	if start < 0 {
		return errors.New("browser tool response contained no result")
	}
	result := text[start+len(resultMarker):]
	if end := strings.Index(result, "\n### "); end >= 0 {
		result = result[:end]
	}
	result = strings.TrimSpace(result)
	if strings.HasPrefix(result, "```json") && strings.HasSuffix(result, "```") {
		result = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(result, "```json"), "```"))
	}
	return json.Unmarshal([]byte(result), target)
}

func appendDirectBrowserURLs(objects, directURLs []string) []string {
	augmented := append([]string{}, objects...)
	for index := range augmented {
		if index >= len(directURLs) {
			break
		}
		directURL := validDirectBrowserURL(directURLs[index])
		if directURL == "" {
			continue
		}
		augmented[index] += " Direct URL: " + directURL
	}
	return augmented
}

func validDirectBrowserURL(value string) string {
	directURL := strings.TrimSpace(value)
	parsed, err := url.Parse(directURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return ""
	}
	return directURL
}

func orientBrowserPerception(instanceRoot string) error {
	const code = `async (page) => {
  const before = await page.evaluate(() => window.scrollY);
  await page.evaluate(() => window.scrollBy(0, Math.max(window.innerHeight * 0.85, 600)));
  await page.waitForTimeout(1500);
  const after = await page.evaluate(() => window.scrollY);
  return {url: page.url(), before, after};
}`
	_, err := callBrowserBody(instanceRoot, "browser_run_code_unsafe", map[string]any{"code": code})
	return err
}

func renewBrowserPerception(instanceRoot string, returnPath []string) (bool, error) {
	target := ""
	if len(returnPath) > 0 {
		target = validNavigableBrowserURL(returnPath[len(returnPath)-1])
	}
	encodedTarget, _ := json.Marshal(target)
	code := fmt.Sprintf(`async (page) => {
  const url = page.url();
	const returnURL = %s;
	if (returnURL && returnURL !== url) {
	  await page.goto(returnURL, {waitUntil: "domcontentloaded"});
	} else {
	  await page.reload({waitUntil: "domcontentloaded"});
	}
  await page.waitForTimeout(2500);
  await page.evaluate(() => window.scrollTo(0, 0));
	return {url, currentUrl: page.url(), returned: Boolean(returnURL && returnURL !== url), renewed: true};
}`, string(encodedTarget))
	_, err := callBrowserBody(instanceRoot, "browser_run_code_unsafe", map[string]any{"code": code})
	return target != "", err
}

func callBrowserBody(instanceRoot, tool string, arguments map[string]any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	helper := filepath.Join(instanceRoot, "body", "bin", "hominal-browser")
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, helper, "call", tool, string(encoded))
	command.Env = append(os.Environ(), "HOMINAL_INSTANCE_ROOT="+instanceRoot)
	return command.Output()
}

func browserSnapshotText(raw []byte) (string, error) {
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(response.Content))
	for _, content := range response.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			parts = append(parts, content.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func browserSemanticSnapshot(snapshot string) string {
	contextLines, objects := browserSemanticObjects(snapshot)
	return browserSemanticSnapshotFromObjects(contextLines, objects)
}

func browserSemanticObjects(snapshot string) ([]string, []string) {
	meta := make([]string, 0, 2)
	articles := make([]string, 0, 8)
	mainContent := make([]string, 0, 20)
	globalHeadings := make([]string, 0, 12)
	scopes := make([]browserSemanticScope, 0, 8)
	seen := make(map[string]bool)
	hasMain := false
	for _, rawLine := range strings.Split(snapshot, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "- Page URL:"):
			meta = append(meta, strings.TrimPrefix(line, "- "))
		case strings.HasPrefix(line, "- Page Title:"):
			meta = append(meta, strings.TrimPrefix(line, "- "))
		}
		if match := browserArticlePattern.FindStringSubmatch(line); len(match) == 2 {
			article := normalizeBrowserSemanticText(match[1])
			if article != "" && !seen[article] && len(articles) < 8 {
				seen[article] = true
				articles = append(articles, article)
			}
			continue
		}

		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " \t"))
		for len(scopes) > 0 && scopes[len(scopes)-1].indent >= indent {
			scopes = scopes[:len(scopes)-1]
		}
		insideMain, insideInterface := browserSemanticPosition(scopes)
		if match := browserNamedObjectPattern.FindStringSubmatch(line); len(match) == 2 {
			semantic := normalizeBrowserSemanticText(match[1])
			if semantic != "" && !seen[semantic] {
				switch {
				case insideMain && !insideInterface && len(mainContent) < 20:
					seen[semantic] = true
					mainContent = append(mainContent, semantic)
				case !insideMain && strings.Contains(line, `heading "`) && !insideInterface && len(globalHeadings) < 12:
					globalHeadings = append(globalHeadings, semantic)
				}
			}
		}
		if role := browserSemanticContainerRole(line); role != "" {
			if role == "main" {
				hasMain = true
			}
			scopes = append(scopes, browserSemanticScope{indent: indent, role: role})
		}
	}
	objects := articles
	if len(articles) > 0 {
	} else if !browserContextUsesArticleObjects(meta) {
		if hasMain {
			objects = mainContent
		} else {
			objects = globalHeadings
		}
	}
	return meta, objects
}

func browserSemanticPosition(scopes []browserSemanticScope) (insideMain, insideInterface bool) {
	for _, scope := range scopes {
		switch scope.role {
		case "main":
			insideMain = true
		case "banner", "contentinfo", "complementary", "navigation", "search", "toolbar", "menubar", "menu", "tablist":
			insideInterface = true
		}
	}
	return insideMain, insideInterface
}

func browserSemanticContainerRole(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	for _, role := range [...]string{"main", "banner", "contentinfo", "complementary", "navigation", "search", "toolbar", "menubar", "menu", "tablist"} {
		if line == role || strings.HasPrefix(line, role+" ") || strings.HasPrefix(line, role+` "`) {
			return role
		}
	}
	return ""
}

func browserContextUsesArticleObjects(contextLines []string) bool {
	for _, line := range contextLines {
		if !strings.HasPrefix(line, "Page URL:") {
			continue
		}
		parsed, err := url.Parse(strings.TrimSpace(strings.TrimPrefix(line, "Page URL:")))
		if err != nil {
			return false
		}
		host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
		// On X, headings and links are predominantly the application's own
		// navigation chrome. Authored feed objects have an article role. Passive
		// perception stays conservative and waits for an article instead of making
		// Alice appraise the eye's keyboard shortcuts, logo or menus as world
		// content. Active browser actions can still inspect every affordance.
		return host == "x.com"
	}
	return false
}

func browserSemanticSnapshotFromObjects(contextLines, objects []string) string {
	parts := append([]string{}, contextLines...)
	if len(objects) > 0 {
		parts = append(parts, "Visible objects:")
		for _, object := range objects {
			parts = append(parts, "- "+object)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func normalizeBrowserSemanticText(text string) string {
	text = relativeTimePattern.ReplaceAllString(text, "<relative-time>")
	text = engagementPattern.ReplaceAllString(text, "<metric> $1")
	text = mediaClockPattern.ReplaceAllString(text, "<media-time>")
	// Player controls, playback position, galleries and engagement counters are
	// mutable presentation state around a post, not a new perceived object. Keep
	// the author and authored content as the stable identity.
	text = mediaSurfacePattern.ReplaceAllString(text, "")
	return strings.Join(strings.Fields(text), " ")
}

func perceptualObjectDigest(object string) string {
	identity := object
	if marker := strings.LastIndex(object, " Direct URL: "); marker >= 0 {
		if direct := validDirectBrowserURL(object[marker+len(" Direct URL: "):]); direct != "" {
			identity = direct
		}
	}
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func queuePerceptualNovelty(previous PerceptualTrace, observation perceptualObservation) PerceptualTrace {
	contextChanged := len(previous.Context) > 0 && perceptualContextKey(previous.Context) != perceptualContextKey(observation.Context)
	if contextChanged {
		previousURL := browserContextURL(previous.Context)
		currentURL := browserContextURL(observation.Context)
		if index := lastURLIndex(previous.ReturnPath, currentURL); index >= 0 {
			// Returning to any earlier surface consumes that surface and every
			// younger descendant. The remaining path still knows older, broader
			// contexts instead of forgetting them after one nested detail.
			previous.ReturnPath = append([]string{}, previous.ReturnPath[:index]...)
		} else if prior := validNavigableBrowserURL(previousURL); prior != "" && prior != currentURL {
			if len(previous.ReturnPath) == 0 || previous.ReturnPath[len(previous.ReturnPath)-1] != prior {
				previous.ReturnPath = append(previous.ReturnPath, prior)
			}
			const maximumReturnDepth = 8
			if len(previous.ReturnPath) > maximumReturnDepth {
				previous.ReturnPath = append([]string{}, previous.ReturnPath[len(previous.ReturnPath)-maximumReturnDepth:]...)
			}
		}
		previous.Pending = nil
		previous.Saturation = 0
		previous.ExhaustedContext = ""
		previous.ExhaustedAt = ""
	}
	known := make(map[string]bool, len(previous.Seen)+len(previous.Pending)+len(observation.Objects))
	for _, digest := range previous.Seen {
		known[digest] = true
	}
	for _, object := range previous.Pending {
		known[perceptualObjectDigest(object)] = true
	}
	pending := append([]string{}, previous.Pending...)
	for _, object := range observation.Objects {
		digest := perceptualObjectDigest(object)
		if known[digest] {
			continue
		}
		known[digest] = true
		pending = append(pending, object)
	}
	return PerceptualTrace{
		Digest:           observation.Digest,
		ObservedAt:       nowUTC(),
		Context:          append([]string{}, observation.Context...),
		Pending:          pending,
		Seen:             append([]string{}, previous.Seen...),
		Saturation:       previous.Saturation,
		ExhaustedContext: previous.ExhaustedContext,
		ExhaustedAt:      previous.ExhaustedAt,
		ReturnPath:       append([]string{}, previous.ReturnPath...),
	}
}

func lastURLIndex(values []string, target string) int {
	target = validNavigableBrowserURL(target)
	if target == "" {
		return -1
	}
	for index := len(values) - 1; index >= 0; index-- {
		if validNavigableBrowserURL(values[index]) == target {
			return index
		}
	}
	return -1
}

func browserContextURL(contextLines []string) string {
	for _, line := range contextLines {
		if strings.HasPrefix(line, "Page URL:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Page URL:"))
		}
	}
	return ""
}

func validNavigableBrowserURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return ""
	}
	return parsed.String()
}

func takePerceptualNovelty(trace PerceptualTrace) (PerceptualTrace, string) {
	if len(trace.Pending) == 0 {
		return trace, ""
	}
	object := trace.Pending[0]
	trace.Pending = append([]string{}, trace.Pending[1:]...)
	trace.Seen = append(trace.Seen, perceptualObjectDigest(object))
	const maximumSeenObjects = 512
	if len(trace.Seen) > maximumSeenObjects {
		trace.Seen = append([]string{}, trace.Seen[len(trace.Seen)-maximumSeenObjects:]...)
	}
	return trace, browserSemanticSnapshotFromObjects(trace.Context, []string{object})
}

func perceptualResampleDue(trace PerceptualTrace, now time.Time, revisitSeconds int) bool {
	contextKey := perceptualContextKey(trace.Context)
	return contextKey != "" && len(trace.Pending) == 0 && perceptualExhaustionDue(trace, contextKey, now, revisitSeconds)
}

func perceptualSaturationDue(trace PerceptualTrace, threshold float64, now time.Time, revisitSeconds int) bool {
	contextKey := perceptualContextKey(trace.Context)
	return contextKey != "" && trace.Saturation >= threshold && perceptualExhaustionDue(trace, contextKey, now, revisitSeconds)
}

func perceptualReturnDue(trace PerceptualTrace) bool {
	contextKey := perceptualContextKey(trace.Context)
	return len(trace.ReturnPath) > 0 && contextKey != "" && trace.ExhaustedContext == contextKey
}

func perceptualExhaustionDue(trace PerceptualTrace, contextKey string, now time.Time, revisitSeconds int) bool {
	if trace.ExhaustedContext != contextKey {
		return true
	}
	if revisitSeconds <= 0 || trace.ExhaustedAt == "" {
		return false
	}
	exhaustedAt, err := time.Parse(time.RFC3339Nano, trace.ExhaustedAt)
	return err != nil || now.Sub(exhaustedAt) >= time.Duration(revisitSeconds)*time.Second
}

func perceptualContextKey(contextLines []string) string {
	if len(contextLines) == 0 {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.Join(contextLines, "\n")))
	return hex.EncodeToString(digest[:])

}

func reopenPerceptualSampling(trace PerceptualTrace) PerceptualTrace {
	trace.Saturation = 0
	trace.ExhaustedContext = ""
	trace.ExhaustedAt = ""
	return trace
}

func discardPendingPerception(trace PerceptualTrace) PerceptualTrace {
	for _, object := range trace.Pending {
		trace.Seen = append(trace.Seen, perceptualObjectDigest(object))
	}
	trace.Pending = nil
	const maximumSeenObjects = 512
	if len(trace.Seen) > maximumSeenObjects {
		trace.Seen = append([]string{}, trace.Seen[len(trace.Seen)-maximumSeenObjects:]...)
	}
	return trace
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
	if previous.CognitiveResourceBand != "" && current.CognitiveResourceBand != "" && previous.CognitiveResourceBand != current.CognitiveResourceBand {
		differences = append(differences, fmt.Sprintf(
			"cognitive resource band changed from %s to %s; %.6f USD remains in the rolling hour and %.6f USD remains in the rolling day",
			previous.CognitiveResourceBand,
			current.CognitiveResourceBand,
			float64(current.CognitiveHourRemainingMicrousd)/float64(microusdPerUSD),
			float64(current.CognitiveDayRemainingMicrousd)/float64(microusdPerUSD),
		))
	}
	if previous.CognitivePriceTableVersion != "" && previous.CognitivePriceTableVersion != current.CognitivePriceTableVersion {
		differences = append(differences, fmt.Sprintf("cognitive price table changed from %s to %s", previous.CognitivePriceTableVersion, current.CognitivePriceTableVersion))
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
	booleanDifference("clash verge running", previous.ClashVergeRunning, current.ClashVergeRunning)
	return differences
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
