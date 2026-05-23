package notify

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sting8k/jump/services/jumpd/internal/store"
)

// NtfyConfig configures the optional daemon-side ntfy delivery channel.
type NtfyConfig struct {
	Enabled     bool
	ServerURL   string
	TopicID     string
	Token       string
	SendDetails bool
}

func workspaceLabel(sess store.Session) string {
	root := strings.TrimSpace(sess.WorkspaceRoot)
	if root == "" {
		root = strings.TrimSpace(sess.Cwd)
	}
	if root == "" {
		return "session"
	}
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "session"
	}
	return base
}

func (r *Router) ntfyConfig() NtfyConfig {
	if r.config.NtfyProvider == nil {
		return NtfyConfig{}
	}
	cfg := r.config.NtfyProvider()
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	cfg.TopicID = strings.Trim(strings.TrimSpace(cfg.TopicID), "/")
	cfg.Token = strings.TrimSpace(cfg.Token)
	return cfg
}

func (r *Router) publishNtfy(p *pendingNotif) {
	cfg := r.ntfyConfig()
	if !cfg.Enabled || cfg.ServerURL == "" || cfg.TopicID == "" {
		return
	}
	body := formatNtfyMessage(p, cfg.SendDetails)
	r.publishNtfyBody(cfg, body)
}

func (r *Router) publishCoalescedNtfy(events []*pendingNotif) {
	cfg := r.ntfyConfig()
	if !cfg.Enabled || cfg.ServerURL == "" || cfg.TopicID == "" || len(events) == 0 {
		return
	}
	body := formatCoalescedNtfyMessage(events)
	r.publishNtfyBody(cfg, body)
}

func (r *Router) publishNtfyBody(cfg NtfyConfig, body string) {
	client := r.config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sendNtfy(ctx, client, cfg, body); err != nil {
			log.Printf("notify: ntfy publish failed: %v", err)
		}
	}()
}

func sendNtfy(ctx context.Context, client *http.Client, cfg NtfyConfig, body string) error {
	endpoint, err := ntfyEndpoint(cfg.ServerURL, cfg.TopicID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", "jump")
	req.Header.Set("Tags", "computer")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy status %d", resp.StatusCode)
	}
	return nil
}

func ntfyEndpoint(serverURL, topicID string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid ntfy server_url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + url.PathEscape(strings.Trim(topicID, "/"))
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func formatNtfyMessage(p *pendingNotif, details bool) string {
	prefix := fmt.Sprintf("[%s] ", ntfyWorkspace(p.workspace))
	if !details {
		switch p.notifType {
		case "finished":
			return prefix + "session finished"
		case "unread":
			return prefix + "new output"
		default:
			return prefix + "session needs attention"
		}
	}

	title := strings.TrimSpace(p.title)
	body := strings.TrimSpace(p.body)
	if body == "" {
		body = "needs attention"
	}
	body = lowerFirst(body)
	if title == "" {
		return prefix + body
	}
	return prefix + title + ": " + body
}

func formatCoalescedNtfyMessage(events []*pendingNotif) string {
	groups := make(map[string]int)
	for _, ev := range events {
		groups[ntfyWorkspace(ev.workspace)]++
	}
	if len(groups) == 1 {
		for workspace, count := range groups {
			return fmt.Sprintf("[%s] %s", workspace, sessionsNeedAttention(count))
		}
	}

	workspaces := make([]string, 0, len(groups))
	for workspace := range groups {
		workspaces = append(workspaces, workspace)
	}
	sort.Strings(workspaces)
	lines := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		lines = append(lines, fmt.Sprintf("[%s] %s", workspace, sessionsNeedAttention(groups[workspace])))
	}
	return strings.Join(lines, "\n")
}

func sessionsNeedAttention(count int) string {
	if count == 1 {
		return "1 session needs attention"
	}
	return fmt.Sprintf("%d sessions need attention", count)
}

func ntfyWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "session"
	}
	return workspace
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
