package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sting8k/jump/packages/adapter"
	"github.com/sting8k/jump/packages/adapter/adapters"
	"github.com/sting8k/jump/packages/paths"
	"github.com/sting8k/jump/services/jumpd/internal/authtoken"
	"github.com/sting8k/jump/services/jumpd/internal/binhash"
	"github.com/sting8k/jump/services/jumpd/internal/clipfile"
	"github.com/sting8k/jump/services/jumpd/internal/config"
	"github.com/sting8k/jump/services/jumpd/internal/conversations"
	"github.com/sting8k/jump/services/jumpd/internal/devcontainers"
	"github.com/sting8k/jump/services/jumpd/internal/discovery"
	"github.com/sting8k/jump/services/jumpd/internal/fscomplete"
	"github.com/sting8k/jump/services/jumpd/internal/hostmetrics"
	"github.com/sting8k/jump/services/jumpd/internal/netauth"
	"github.com/sting8k/jump/services/jumpd/internal/notify"
	"github.com/sting8k/jump/services/jumpd/internal/peering"
	"github.com/sting8k/jump/services/jumpd/internal/presence"
	"github.com/sting8k/jump/services/jumpd/internal/projects"
	"github.com/sting8k/jump/services/jumpd/internal/relayclient"
	"github.com/sting8k/jump/services/jumpd/internal/sessionfiles"
	"github.com/sting8k/jump/services/jumpd/internal/sessionmeta"
	"github.com/sting8k/jump/services/jumpd/internal/sessionmetrics"
	"github.com/sting8k/jump/services/jumpd/internal/sleep"
	"github.com/sting8k/jump/services/jumpd/internal/store"
	"github.com/sting8k/jump/services/jumpd/internal/tsauth"
	"github.com/sting8k/jump/services/jumpd/internal/tsdiscovery"
	"github.com/sting8k/jump/services/jumpd/internal/unixipc"
	"github.com/sting8k/jump/services/jumpd/internal/update"
	"github.com/sting8k/jump/services/jumpd/internal/webprefs"
	"github.com/sting8k/jump/services/jumpd/internal/wsproxy"
	"nhooyr.io/websocket"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

type LaunchConfig struct {
	DefaultLauncher string             `json:"default_launcher"`
	Launchers       []adapter.Launcher `json:"launchers"`
}

// discoverLaunchers derives launchers from the compiled adapter set and keeps
// only the adapters that are available on this machine.
func discoverLaunchers() LaunchConfig {
	adapterList := append([]adapter.Adapter{}, adapters.All...)
	adapterList = append(adapterList, adapters.DefaultFallback())

	availableByName := discoverAvailableAdapters(adapterList)
	launchers := launchersForAdapters(adapterList, availableByName)

	log.Printf("launchers: discovered %d adapter(s): %v", len(launchers), launcherStates(launchers))
	return LaunchConfig{
		DefaultLauncher: "shell",
		Launchers:       launchers,
	}
}

func discoverAvailableAdapters(adapterList []adapter.Adapter) map[string]bool {
	availableByName := make(map[string]bool, len(adapterList))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, a := range adapterList {
		a := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			available := a.Discover()
			mu.Lock()
			availableByName[a.Name()] = available
			mu.Unlock()
		}()
	}
	wg.Wait()

	return availableByName
}

func launchersForAdapters(adapterList []adapter.Adapter, availableByName map[string]bool) []adapter.Launcher {
	var launchers []adapter.Launcher
	seen := map[string]struct{}{}

	for _, a := range adapterList {
		launchable, ok := a.(adapter.Launchable)
		if !ok {
			continue
		}
		for _, l := range launchable.Launchers() {
			if _, ok := seen[l.ID]; ok {
				continue
			}
			if !availableByName[a.Name()] {
				continue
			}
			seen[l.ID] = struct{}{}
			l.Available = true
			launchers = append(launchers, l)
		}
	}

	return launchers
}

// resolveJump finds the jump binary.
// Priority: sibling to this binary > PATH lookup.
// Both jumpd and jump are always installed to the same directory.
func resolveJump() string {
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "jump")
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
	}
	if p, err := exec.LookPath("jump"); err == nil {
		return p
	}
	return ""
}

func launcherStates(ls []adapter.Launcher) []string {
	states := make([]string, len(ls))
	for i, l := range ls {
		state := "unavailable"
		if l.Available {
			state = "available"
		}
		states[i] = fmt.Sprintf("%s(%s)", l.ID, state)
	}
	return states
}

// launchJump starts a detached jump process with the given command and cwd.
// Returns the PID on success.
// filterEnvPrefix returns env with any variable starting with prefix removed.
func filterEnvPrefix(env []string, prefix string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// launchJump forks a jump runner with the given command and cwd.
//
// resumeID, when non-empty, is passed to the runner via
// JUMP_RESUME_ID so the runner uses the daemon-supplied id
// instead of generating a fresh one. /v1/launch leaves it empty
// (fresh sessions get a runner-generated id); /v1/resume and
// /v1/restart pass the existing session's id so identity (and the
// scrollback directory on disk) carry across the seam. See
// ADR 0003.
//
// JUMP_RESUME_ID is dedicated to this directive and distinct from
// the JUMP_SESSION_ID the runner exports to its child process; a
// nested `jump foo` inherits JUMP_SESSION_ID from the parent
// runner but never JUMP_RESUME_ID, so nested invocations always
// generate a fresh id.
func launchJump(jumpBin string, command []string, cwd, resumeID string) (int, error) {
	cmd := exec.Command(jumpBin, command...)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Strip all JUMP_* session vars so child processes don't inherit
	// the parent session's identity. Without this, a jumpd started
	// inside a pi session would leak JUMP_ADAPTER=pi, JUMP_SOCKET,
	// JUMP_SESSION_ID, etc. into every launched session.
	cmd.Env = filterEnvPrefix(os.Environ(), "JUMP_")
	if resumeID != "" {
		cmd.Env = append(cmd.Env, "JUMP_RESUME_ID="+resumeID)
	}

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go cmd.Wait()
	return cmd.Process.Pid, nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `jumpd %s

Usage: jumpd <command>

Commands:
  start              Start the daemon in the background
  run                Run the daemon in the foreground (for systemd/Docker)
  stop               Stop the running daemon
  restart            Restart the daemon (alias for start)
  status             Show daemon health, listeners, and sessions
  auth               Show the auth URL and token
  tsnet              Set up or check Tailscale/tsnet access
  relay              Set up or check relay access
  doctor             Diagnose config, daemon, and remote access
  log-path           Print the daemon log file path
  version            Show jumpd version
  help               Show this help

Tip:
  jump <command>     Run a command; jump auto-starts jumpd if needed
  listen = "0.0.0.0" in host.toml exposes the TCP UI beyond localhost
  JUMPD_LISTEN=... overrides host.toml for systemd/Docker deployments
  More help: https://github.com/sting8k/jump
`, version)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "start", "restart":
		for _, arg := range args {
			switch arg {
			case "-h", "--help":
				_, _ = fmt.Fprintf(stdout, "Usage: jumpd %s\n\nStarts the daemon in the background, replacing any existing instance.\n", cmd)
				return 0
			default:
				_, _ = fmt.Fprintf(stderr, "jumpd %s: unknown option %q\n", cmd, arg)
				return 2
			}
		}
		return startBackground(stdout, stderr)
	case "run":
		for _, arg := range args {
			switch arg {
			case "-h", "--help":
				_, _ = fmt.Fprintf(stdout, "Usage: jumpd run\n\nRuns the daemon in the foreground (for systemd, Docker, or debugging).\n")
				return 0
			default:
				_, _ = fmt.Fprintf(stderr, "jumpd run: unknown option %q\n", arg)
				return 2
			}
		}
		return serve(stderr)
	case "stop":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd stop: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		sock := paths.SocketPath()
		if unixipc.Shutdown(sock) {
			_, _ = fmt.Fprintf(stdout, "jumpd: stopped\n")
		} else {
			_, _ = fmt.Fprintf(stdout, "jumpd: no running daemon found\n")
		}
		return 0
	case "status":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd status: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		return runStatus(stdout, stderr)
	case "auth":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd auth: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		return runAuth(stdout, stderr)
	case "tsnet":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd tsnet: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		return runTsnet(os.Stdin, stdout, stderr)
	case "relay":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd relay: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		return runRelay(os.Stdin, stdout, stderr)
	case "doctor":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd doctor: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		return runDoctor(stdout, stderr)
	case "version":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd version: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", version)
		return 0
	case "log-path":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "jumpd log-path: unexpected arguments: %s\n", strings.Join(args, " "))
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "%s\n", filepath.Join(paths.StateDir(), "jumpd.log"))
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "jumpd: unknown command %q\n\n", cmd)
		printUsage(stderr)
		return 2
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// startBackground re-execs jumpd with "run" in a detached process,
// replacing any existing daemon. Output goes to a log file in the
// state directory. Waits briefly to confirm startup succeeded.
func startBackground(stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: cannot determine own path: %v\n", err)
		return 1
	}

	// Check for an existing daemon and stop it first.
	sock := paths.SocketPath()
	replaced := false
	if oldVer, ok := unixipc.HealthVersion(sock); ok {
		replaced = true
		if oldVer != "" {
			_, _ = fmt.Fprintf(stdout, "jumpd: stopping existing daemon (%s)...\n", oldVer)
		} else {
			_, _ = fmt.Fprintf(stdout, "jumpd: stopping existing daemon...\n")
		}
		if !unixipc.Shutdown(sock) {
			_, _ = fmt.Fprintf(stderr, "jumpd: existing daemon did not shut down\n")
			return 1
		}
	}

	stateDir := paths.StateDir()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: cannot create state dir %s: %v\n", stateDir, err)
		return 1
	}

	logPath := filepath.Join(stateDir, "jumpd.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: cannot open log %s: %v\n", logPath, err)
		return 1
	}

	cmd := exec.Command(exe, "run")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = logFile
	cmd.Stdin = nil
	// Strip JUMP_* env vars so the daemon doesn't inherit session identity.
	cmd.Env = filterEnvPrefix(os.Environ(), "JUMP_")

	if err := cmd.Start(); err != nil {
		logFile.Close()
		_, _ = fmt.Fprintf(stderr, "jumpd: failed to start: %v\n", err)
		return 1
	}
	go func() {
		cmd.Wait()
		logFile.Close()
	}()

	// Wait for the daemon to become healthy.
	healthy := false
	for range 30 {
		time.Sleep(100 * time.Millisecond)
		if unixipc.Healthy(sock) {
			healthy = true
			break
		}
	}

	if !healthy {
		_, _ = fmt.Fprintf(stderr, "jumpd: started (pid %d); local daemon is still starting\n  This is startup readiness, not tsnet/relayd health. Check with `jumpd status` or `jumpd doctor`.\n  Logs: %s\n", cmd.Process.Pid, logPath)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "jumpd: running %s (pid %d)\n  Logs: %s\n", version, cmd.Process.Pid, logPath)
	if replaced {
		_, _ = fmt.Fprintf(stdout, "  Note: active sessions will use the new version when restarted.\n")
	}
	return 0
}

func serve(stderr io.Writer) int {
	jumpBin := resolveJump() // resolve once, use everywhere
	if jumpBin != "" {
		log.Printf("jump: %s", jumpBin)
		h := binhash.File(jumpBin)
		if h != "" {
			discovery.ExpectedRunnerHash = h
			log.Printf("jump hash: %s…", h[:12])
		}
	}
	launchConfig := discoverLaunchers()

	sessions := store.New()

	// sessionmeta persists per-session records so dead sessions
	// survive a jumpd restart. Sweep on startup repopulates the
	// store with everything we knew about previously; the OnDead
	// hook below persists every Alive=false landing; Dismiss /
	// Resume merge / slug takeover drop the corresponding directory.
	// See sessionmeta package doc for the full lifecycle.
	metaStore := sessionmeta.New(sessionmeta.DefaultDir())
	if loaded, err := metaStore.Sweep(); err != nil {
		log.Printf("sessionmeta: sweep failed: %v", err)
	} else {
		for _, sess := range loaded {
			sessions.Upsert(sess)
		}
		if n := len(loaded); n > 0 {
			log.Printf("sessionmeta: restored %d session(s) from %s", n, metaStore.Dir())
		}
	}
	persistDead := func(sess store.Session) {
		if err := metaStore.Write(sess); err != nil {
			log.Printf("sessionmeta: write %s: %v", sess.ID, err)
		}
	}
	forgetMeta := func(id string) {
		if err := metaStore.Remove(id); err != nil {
			log.Printf("sessionmeta: remove %s: %v", id, err)
		}
	}

	// Drive the persister's removal loop off store events so every
	// session-remove (dismiss, slug takeover, peer disconnect, etc.)
	// drops the matching meta dir. The explicit forgetMeta call in
	// the dismiss handler is redundant but cheap. Resume is an
	// alive=false→true Upsert under the same id (ADR 0003) and
	// leaves meta.json in place; it gets overwritten by persistDead
	// the next time the session dies, or harmlessly rediscovered as
	// alive=true on the next daemon restart. No explicit cleanup
	// needed.
	metaEvents, cancelMetaEvents := sessions.Subscribe()
	defer cancelMetaEvents()
	go metaStore.WatchRemovals(metaEvents)

	// Build command titlers from adapters that implement CommandTitler.
	commandTitlers := make(map[string]func([]string) string)
	for _, a := range adapters.AllAdapters() {
		if ct, ok := a.(adapter.CommandTitler); ok {
			ct := ct // capture for closure
			commandTitlers[a.Name()] = ct.CommandTitle
		}
	}
	sessions.SetCommandTitlers(commandTitlers)

	subs := discovery.NewSubscriptions(sessions)
	subs.OnDead = persistDead
	var resumeMu sync.Mutex

	// Start file monitor — watches adapter session directories with inotify
	// to extract title and working status from JSONL files.
	fileMon := discovery.NewFileMonitor(sessions)

	// When a session exits, derive the resume command so it transitions
	// to resumable immediately — no "exited" limbo state.
	subs.OnExit = func(sess *store.Session) bool {
		if cmd := fileMon.ResolveResumeCommand(sess); cmd != nil {
			sess.Command = cmd
			sess.Status = nil // clear exit status for clean resumable display
			return true
		}
		return false
	}
	stopFileMon := make(chan struct{})
	go fileMon.Run(stopFileMon)
	defer close(stopFileMon)

	// Start socket-based discovery (scans /tmp/jump-sessions/*.sock)
	// Discovery also subscribes to each runner's /events SSE for live updates.
	discoveryFirstScan := make(chan struct{})
	stopDiscovery := make(chan struct{})
	go discovery.Watch(sessions, subs, fileMon, persistDead, func() {
		fileMon.ApplyPersistedAttributions()
		close(discoveryFirstScan)
	}, 3*time.Second, stopDiscovery)
	defer close(stopDiscovery)

	// Session file scanner — discovers resumable sessions from adapter
	// session files (e.g. pi's JSONL conversations). Also purges stale
	// dead sessions that were never attributed to a file. Started below
	// after the project manager is set up so the first-scan callback
	// can clean up orphaned project session refs.
	scanner := sessionfiles.New(sessions)
	stopScanner := make(chan struct{})
	defer close(stopScanner)

	// Conversations index — maps (kind, slug) to file metadata for URL
	// resolution of dead conversations and future fulltext search.
	// One bootstrap scan at startup; from then on the index is kept
	// fresh by filemon's fsnotify event handler (SetConvIndex below).
	convIndex := conversations.New()
	convIndex.Scan()
	log.Printf("conversations: indexed %d files", convIndex.Count())

	// Wire filemon to the conversations index and install always-on
	// watches on every adapter session root. After this, every .jsonl
	// Create/Write/Remove under any adapter root updates the index
	// automatically, with no periodic scan involved.
	fileMon.SetConvIndex(convIndex)
	fileMon.WatchRoots()

	// Start background update checker
	updateChecker := update.New(version)

	// State directory for persistent files (projects.json, auth-token, web preferences, etc).
	stateDir := paths.StateDir()

	// Web preferences are jumpd-managed UI state, separate from user-editable
	// settings.jsonc/theme.jsonc terminal configuration.
	webPrefsMgr := webprefs.NewManager(stateDir)

	// ── Presence + Notification router ──

	notifRouter := (*notify.Router)(nil) // assigned after presence table
	presenceTable := presence.New(presence.Callbacks{
		OnClientFocused: func(clientID string) {
			if notifRouter != nil {
				notifRouter.CancelAll()
			}
		},
		OnSessionSelected: func(clientID, sessionID string) {
			if notifRouter != nil {
				notifRouter.CancelForSession(sessionID)
			}
		},
	})
	notifConfig := notify.DefaultConfig()
	notifConfig.NtfyProvider = func() notify.NtfyConfig {
		prefs, err := webPrefsMgr.Load()
		if err != nil {
			log.Printf("notify: ntfy preferences unavailable: %v", err)
			return notify.NtfyConfig{}
		}
		ntfy := prefs.Notifications.Ntfy
		return notify.NtfyConfig{
			Enabled:     ntfy.Enabled,
			ServerURL:   ntfy.ServerURL,
			TopicID:     ntfy.TopicID,
			Token:       ntfy.Token,
			SendDetails: ntfy.SendDetails,
		}
	}
	notifRouter = notify.New(presenceTable, sessions, notifConfig)
	notifCtx, notifCancel := context.WithCancel(context.Background())
	go notifRouter.Run(notifCtx)
	defer notifCancel()

	mux := http.NewServeMux()

	// tsListener is set below if tailscale is enabled. Declared here so
	// the health handler can include the tailscale URL.
	var tsListener *tsauth.Listener

	// tcpAddr and authToken are resolved after config load. Declared here
	// so the health handler can report the address.
	var tcpAddr string
	var authToken string

	// Project manager handles concurrent access to projects.json and
	// auto-assignment of sessions to projects.
	projectMgr := projects.NewManager(stateDir)
	projectMgr.Broadcast = func() {
		sessions.Broadcast(store.Event{Type: "projects-update"})
	}
	projectMgr.SeedIfEmpty()

	// Keep project membership arrays in sync with scanner-driven removals
	// such as stale ephemeral cleanup, 24-hour dead-session TTL pruning, and invalid timestamp cleanup.
	scanner.OnRemove = func(sess store.Session) {
		projectMgr.DismissSession(sess.ID, sess.Slug)
	}

	// Populate the store with project-tracked sessions that don't have
	// a sessionmeta record. The sessionmeta sweep above is the SOT for
	// runtime fields; this only fills in the pre-S2 fallback path. See
	// rehydrateProjects for the identity-model rationale.
	if state, err := projectMgr.Load(); err == nil {
		rehydrateProjects(sessions, convIndex, state)
	}

	// Release transient heap from startup history scans. convIndex keeps only
	// metadata; the large temporary JSONL buffers used during adapter parsing
	// should not keep jumpd's steady-state RSS high after boot.
	debug.FreeOSMemory()

	// After store is populated, clean up orphaned project entries
	// (slugs that no longer resolve to a store session).
	scanner.OnFirstScan = func() {
		known := make(map[string]bool)
		for _, s := range sessions.List() {
			known[s.ID] = true
			if s.Slug != "" {
				known[s.Slug] = true
			}
		}
		projectMgr.CleanupSessions(known)
	}
	// Delay the retention scanner until discovery's initial scan has had a chance
	// to re-register still-live runners restored from sessionmeta as Alive=false.
	go runAfterDiscoveryFirstScan(discoveryFirstScan, stopScanner, func() {
		scanner.Run(30*time.Second, stopScanner)
	})

	// Conversations index updates are watcher-driven via filemon
	// (see SetConvIndex + WatchRoots above). No periodic rescan: a
	// healthy fsnotify watch tree plus the startup bootstrap scan
	// covers steady state. If reports of staleness emerge after
	// suspend or inotify queue overflow, add an explicit reconcile
	// hook — don't reintroduce the periodic ticker.

	// Auto-assign sessions to projects when they appear or get a Slug.
	sessionEvents, unsubSessionEvents := sessions.Subscribe()
	defer unsubSessionEvents()
	go func() {
		for ev := range sessionEvents {
			if ev.Type != "session-upsert" || ev.Session == nil {
				continue
			}
			s := ev.Session
			// Only auto-assign alive sessions. Dead resumable sessions
			// stay in the array if already persisted from a previous run,
			// but we don't bulk-add hundreds of old session files on startup.
			if !s.Alive {
				continue
			}
			projectMgr.AutoAssignSession(projects.SessionInfo{
				ID:            s.ID,
				Cwd:           s.Cwd,
				WorkspaceRoot: s.WorkspaceRoot,
				Remotes:       s.Remotes,
				Host:          s.Peer,
				Alive:         s.Alive,
				Slug:          s.Slug,
			})
		}
	}()

	// peerManager and tsDiscovery are initialized later after config is
	// loaded. The closures capture the pointers so handlers work once set.
	var peerManager *peering.Manager
	var remoteMode string
	var relayStatus map[string]string
	var tsDiscovery *tsdiscovery.Watcher

	// ── Health + Capabilities ──

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"service": "jumpd",
			"version": version,
			"node_id": "node-local",
			"status":  "ready",
		}
		if h, err := os.Hostname(); err == nil {
			data["hostname"] = h
		}
		if remoteMode != "" {
			data["remote_mode"] = remoteMode
		}
		if relayStatus != nil {
			data["relay"] = relayStatus
		}
		if tsListener != nil {
			diag := tsListener.Diag()
			if diag.FQDN != "" {
				data["tailscale_url"] = "https://" + diag.FQDN
			}
			data["tailscale"] = diag
		}
		data["listen"] = tcpAddr
		if v := updateChecker.Available(); v != "" {
			data["update_available"] = v
		}
		// Include auth token only on Unix socket connections (local IPC).
		// On TCP, the requester already proved they have the token.
		if r.RemoteAddr == "@" || strings.HasPrefix(r.RemoteAddr, "/") || r.RemoteAddr == "" {
			data["auth_token"] = authToken
		}
		if peers := appendOfflinePeers(peerManager, tsDiscovery); len(peers) > 0 {
			data["peers"] = peers
		}

		// Session summary.
		all := sessions.List()
		var localAlive, remoteAlive, dead int
		for _, s := range all {
			switch {
			case !s.Alive:
				dead++
			case s.Peer == "":
				localAlive++
			default:
				remoteAlive++
			}
		}
		data["sessions"] = map[string]int{
			"local_alive":  localAlive,
			"remote_alive": remoteAlive,
			"dead":         dead,
		}

		// runner_hash is the sha256 of the jump runner binary on disk.
		// The frontend uses this (alongside runner_version on sessions)
		// to detect dev-mode builds where both sides report "dev" but
		// were compiled from different commits.
		if discovery.ExpectedRunnerHash != "" {
			data["runner_hash"] = discovery.ExpectedRunnerHash
		}

		// Launchers: what adapters can be launched on this host.
		data["default_launcher"] = launchConfig.DefaultLauncher
		data["launchers"] = launchConfig.Launchers

		writeJSON(w, map[string]any{"ok": true, "data": data})
	})

	registerHostActionRoutes(mux, defaultHostActionService{})

	mux.HandleFunc("GET /v1/host-metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics, err := hostmetrics.Collect(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "metrics_failed", err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "data": metrics})
	})

	mux.HandleFunc("GET /v1/session-metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics := sessionmetrics.Collect(r.Context(), sessions.List())
		writeJSON(w, map[string]any{"ok": true, "data": metrics})
	})

	mux.HandleFunc("GET /v1/fs/complete", func(w http.ResponseWriter, r *http.Request) {
		items, err := fscomplete.Complete(r.URL.Query().Get("path"), 25)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "data": items})
	})

	mux.HandleFunc("/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"adapters": []string{"pi", "shell"},
				"transport": map[string]any{
					"kind":   "websocket",
					"replay": true,
				},
			},
		})
	})

	registerFrontendConfigRoutes(mux, webPrefsMgr)

	// ── Projects ──

	mux.HandleFunc("GET /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		state, err := projectMgr.Load()
		if err != nil {
			log.Printf("projects: load error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to load projects")
			return
		}

		sessionInfos := buildSessionInfos(sessions)

		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"configured":             state.Items,
				"discovered":             state.Discovered(sessionInfos),
				"unmatched_active_count": state.UnmatchedActiveCount(sessionInfos),
			},
		})
	})

	mux.HandleFunc("PUT /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var incoming projects.State
		if err := json.Unmarshal(body, &incoming); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}
		if err := incoming.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		err = projectMgr.Update(func(state *projects.State) bool {
			*state = incoming
			return true
		})
		if err != nil {
			log.Printf("projects: save error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to save projects")
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/projects/add", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var req struct {
			Remote string   `json:"remote"`
			Paths  []string `json:"paths"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}

		if len(req.Paths) == 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "paths required")
			return
		}

		// Build match rules from the request.
		var rules []projects.MatchRule
		if req.Remote != "" {
			rules = append(rules, projects.MatchRule{
				Remote: projects.NormalizeRemote(req.Remote),
			})
		}
		for _, p := range req.Paths {
			rules = append(rules, projects.MatchRule{
				Path: paths.CanonicalizePath(p),
			})
		}

		// Derive slug: prefer remote repo name, fall back to first path basename.
		var slug string
		if req.Remote != "" {
			slug = projects.SlugFromRemote(req.Remote)
		} else {
			slug = projects.SlugFromPath(req.Paths[0])
		}

		var item projects.Item
		var validationErr error
		err = projectMgr.Update(func(state *projects.State) bool {
			slug = projects.UniqueSlug(slug, state.Items)
			item = projects.Item{
				Slug:  slug,
				Match: rules,
			}
			state.Items = append(state.Items, item)
			if err := state.Validate(); err != nil {
				validationErr = err
				return false
			}
			return true
		})
		if validationErr != nil {
			writeError(w, http.StatusBadRequest, "validation_error", validationErr.Error())
			return
		}
		if err != nil {
			log.Printf("projects: add error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to save projects")
			return
		}
		// Populate the new project's sessions array with alive matches
		// immediately, so the frontend sees them on the first fetch.
		projectMgr.AutoAssignAllAlive(buildSessionInfos(sessions))
		writeJSON(w, map[string]any{"ok": true, "data": item})
	})

	mux.HandleFunc("PATCH /v1/projects/{slug}/sessions", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}
		var req struct {
			Sessions []string `json:"sessions"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}
		found := false
		err = projectMgr.Update(func(state *projects.State) bool {
			for i := range state.Items {
				if state.Items[i].Slug == slug {
					state.Items[i].Sessions = req.Sessions
					found = true
					return true
				}
			}
			return false
		})
		if err != nil {
			log.Printf("projects: reorder sessions error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to save projects")
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "not_found", "project not found")
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	// ── Sessions ──

	mux.HandleFunc("GET /v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "data": sessions.List()})
	})

	// Conversation lookup — resolve dead conversations by (kind, slug)
	// for URL resolution. Returns file metadata + resume command.
	mux.HandleFunc("GET /v1/conversations/{kind}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		kind := r.PathValue("kind")
		slug := r.PathValue("slug")
		info, ok := convIndex.Lookup(kind, slug)
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"slug":           info.Slug,
				"kind":           info.Kind,
				"title":          info.Title,
				"cwd":            info.Cwd,
				"resume_command": info.ResumeCommand,
				"created":        info.Created,
			},
		})
	})

	// ── Registration (fast path for jump-run) ──

	mux.HandleFunc("POST /v1/register", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var req struct {
			SessionID  string `json:"session_id"`
			SocketPath string `json:"socket_path"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}

		if req.SessionID == "" || req.SocketPath == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "session_id and socket_path required")
			return
		}

		log.Printf("register: %s at %s", req.SessionID, req.SocketPath)
		if err := discovery.Register(sessions, subs, fileMon, req.SocketPath, persistDead); err != nil {
			log.Printf("register: failed to query meta for %s: %v", req.SessionID, err)
			writeError(w, http.StatusBadGateway, "runner_unreachable", err.Error())
			return
		}

		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/deregister", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}

		// Don't remove from store — the exit event from the subscription
		// already marked it alive: false. Just clean up the subscription.
		subs.Unsubscribe(req.SessionID)
		log.Printf("deregister: %s", req.SessionID)
		writeJSON(w, map[string]any{"ok": true})
	})

	// ── Launch ──

	mux.HandleFunc("POST /v1/launch", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "read error")
			return
		}

		var req struct {
			Cwd        string   `json:"cwd"`
			Command    []string `json:"command"`
			LauncherID string   `json:"launcher_id"`
			Peer       string   `json:"peer"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}

		// Forward to peer if requested. ForwardLaunch strips the peer
		// field from the body so the spoke treats it as a local launch.
		if req.Peer != "" {
			if peerManager == nil {
				writeError(w, http.StatusBadRequest, "unknown_peer", "no peers configured")
				return
			}
			if peer := peerManager.GetPeer(req.Peer); peer != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				peer.ForwardLaunch(w, r)
				return
			}
			writeError(w, http.StatusBadRequest, "unknown_peer", fmt.Sprintf("peer %q not configured", req.Peer))
			return
		}

		// Resolve command from launcher_id if no explicit command.
		if len(req.Command) == 0 && req.LauncherID != "" {
			cfg := launchConfig
			found := false
			for _, l := range cfg.Launchers {
				if l.ID == req.LauncherID {
					req.Command = l.Command
					found = true
					break
				}
			}
			if !found {
				writeError(w, http.StatusBadRequest, "launcher_unavailable", fmt.Sprintf("launcher %q is not available on this system", req.LauncherID))
				return
			}
		}

		// Empty/nil command means "shell" — use user's $SHELL
		if len(req.Command) == 0 {
			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "/bin/sh"
			}
			req.Command = []string{shell}
		}

		cwd := req.Cwd
		if cwd == "" {
			cwd = os.Getenv("HOME")
		}
		// Expand ~ to absolute path for exec.Command.Dir.
		cwd = projects.NormalizePath(cwd)

		if jumpBin == "" {
			writeError(w, http.StatusInternalServerError, "jump_not_found", "jump not found (install jump alongside jumpd)")
			return
		}

		pid, err := launchJump(jumpBin, req.Command, cwd, "")
		if err != nil {
			log.Printf("launch: failed to start jump: %v", err)
			writeError(w, http.StatusInternalServerError, "launch_failed", err.Error())
			return
		}

		log.Printf("launch: started jump pid=%d cwd=%s cmd=%v", pid, cwd, req.Command)
		writeJSON(w, map[string]any{
			"ok":   true,
			"data": map[string]any{"pid": pid},
		})
	})

	// ── Session Actions ──

	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		sessionID := parts[2]
		action := ""
		if len(parts) == 4 {
			action = parts[3]
		}

		// Route to peer if this is a remote session.
		if peerManager != nil && action != "" {
			if peer, originalID := peerManager.FindPeer(sessionID); peer != nil {
				if action == "attach" {
					// Attach returns the hub's own WS path (the hub proxies to the spoke).
					writeJSON(w, map[string]any{
						"ok": true,
						"data": map[string]any{
							"transport": "websocket",
							"ws_path":   "/ws/" + sessionID,
						},
					})
					return
				}
				peer.Forward(w, r, originalID, action)
				return
			}
		}

		switch action {
		case "attach":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			sess, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			writeJSON(w, map[string]any{
				"ok": true,
				"data": map[string]any{
					"transport":   "websocket",
					"ws_path":     "/ws/" + sessionID,
					"socket_path": sess.SocketPath,
				},
			})

		case "resume":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			// Serialize resume attempts to prevent double-click races.
			resumeMu.Lock()
			defer resumeMu.Unlock()

			sess, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			if sess.Alive || len(sess.Command) == 0 {
				writeError(w, http.StatusBadRequest, "not_resumable", "session is not resumable")
				return
			}
			if jumpBin == "" {
				writeError(w, http.StatusInternalServerError, "jump_not_found", "jump not found")
				return
			}

			// The runner reads JUMP_RESUME_ID and registers under the
			// same id, so Register() lands in its re-registration
			// branch and the session keeps its identity (and its
			// scrollback directory). See ADR 0003.
			resumeCwd := projects.NormalizePath(sess.Cwd)
			pid, err := launchJump(jumpBin, sess.Command, resumeCwd, sessionID)
			if err != nil {
				log.Printf("resume: failed to start jump: %v", err)
				writeError(w, http.StatusInternalServerError, "launch_failed", err.Error())
				return
			}

			// Don't modify the session here. It stays dead/resumable
			// until the runner calls POST /register and the
			// re-registration upsert flips alive=true.
			// The frontend shows a local "resuming" indicator.
			log.Printf("resume: started jump pid=%d for %s cwd=%s", pid, sessionID, resumeCwd)
			writeJSON(w, map[string]any{
				"ok":   true,
				"data": map[string]any{"pid": pid, "session_id": sessionID},
			})

		case "restart":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			// Serialize with /resume to prevent double-click races.
			resumeMu.Lock()
			defer resumeMu.Unlock()

			sess, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			if jumpBin == "" {
				writeError(w, http.StatusInternalServerError, "jump_not_found", "jump not found")
				return
			}

			// If the runner is alive, kill it and wait for the exit lifecycle
			// to transition the session to resumable (Alive=false + resume Command).
			if sess.Alive {
				if sess.SocketPath == "" {
					writeError(w, http.StatusBadRequest, "no_socket", "alive session missing socket")
					return
				}
				// Subscribe BEFORE killing so we don't miss the exit upsert.
				evCh, unsub := sessions.Subscribe()
				defer unsub()
				if err := discovery.KillSession(sess.SocketPath); err != nil {
					log.Printf("restart: %s: kill failed: %v", sessionID, err)
					writeError(w, http.StatusInternalServerError, "kill_failed", err.Error())
					return
				}
				deadline := time.After(5 * time.Second)
				ready := false
				for !ready {
					select {
					case <-deadline:
						writeError(w, http.StatusGatewayTimeout, "kill_timeout", "session did not exit in time")
						return
					case ev := <-evCh:
						if ev.ID != sessionID || ev.Session == nil {
							continue
						}
						if !ev.Session.Alive && len(ev.Session.Command) > 0 {
							sess = *ev.Session
							ready = true
						}
					}
				}
				// /kill releases the canonical socket path before
				// responding 204, so by the time KillSession returned
				// (above) the path was already free. The replacement
				// runner's BindSocket below cannot race against the old
				// runner's lingering listener for path ownership.
			}

			if sess.Alive || len(sess.Command) == 0 {
				writeError(w, http.StatusBadRequest, "not_resumable", "session is not resumable")
				return
			}

			// Same as /resume: launch a new runner under the existing
			// session id; Register's re-registration branch handles
			// the rest.
			restartCwd := projects.NormalizePath(sess.Cwd)
			pid, err := launchJump(jumpBin, sess.Command, restartCwd, sessionID)
			if err != nil {
				log.Printf("restart: failed to start jump: %v", err)
				writeError(w, http.StatusInternalServerError, "launch_failed", err.Error())
				return
			}
			log.Printf("restart: started jump pid=%d for %s cwd=%s", pid, sessionID, restartCwd)
			writeJSON(w, map[string]any{
				"ok":   true,
				"data": map[string]any{"pid": pid, "session_id": sessionID},
			})

		case "kill":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			sess, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			// Send kill to runner — it will SIGHUP the child, which triggers
			// normal exit lifecycle (exit event → subscription updates store).
			// The live runner socket is the runtime source of truth: even if the
			// store currently says dead, a reachable socket still represents an
			// orphan runner that this endpoint must be able to clean up.
			if sess.SocketPath != "" {
				if err := discovery.KillSession(sess.SocketPath); err != nil {
					log.Printf("kill: %s: runner unreachable, forcing dead: %v", sessionID, err)
					sess.Alive = false
					sess.Status = nil
					if fileMon != nil {
						if cmd := fileMon.ResolveResumeCommand(&sess); cmd != nil {
							sess.Command = cmd
						}
					}
					sessions.Upsert(sess)
					persistDead(sess)
					subs.Unsubscribe(sessionID)
					if fileMon != nil {
						fileMon.NotifySessionDied(sessionID)
					}
					os.Remove(sess.SocketPath)
				}
			}
			writeJSON(w, map[string]any{"ok": true, "data": map[string]any{}})

		case "read":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			sessions.Update(sessionID, func(sess *store.Session) {
				sess.Unread = false
				if sess.Status != nil && sess.Status.Error {
					sess.Status.Error = false
				}
			})
			writeJSON(w, map[string]any{"ok": true, "data": map[string]any{}})

		case "scrollback":
			scrollbackBrokerHandler(w, r, sessionID, sessions, metaStore.SessionDir)

		case "clipboard":
			// Materialize a clipboard binary payload as a file in this
			// jumpd's os.TempDir() and return the absolute path. For
			// devcontainer/peer sessions, the request was already
			// forwarded above to the jumpd that owns the session, so
			// reaching this branch always means "write locally". The
			// session must exist; we don't otherwise need fields from it.
			if _, ok := sessions.Get(sessionID); !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			clipboardHandler(clipfile.NewLocalWriter(os.TempDir())).ServeHTTP(w, r)

		case "dismiss":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			sess, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			// Kill if still alive.
			if sess.SocketPath != "" && sess.Alive {
				if err := discovery.KillSession(sess.SocketPath); err != nil {
					log.Printf("dismiss: %s: runner kill failed: %v", sessionID, err)
				}
			}
			// Let the adapter perform any cleanup (e.g. removing a state file).
			if a := adapters.FindByKind(sess.Kind); a != nil {
				if fin, ok := a.(adapter.SessionFinalizer); ok {
					fin.OnDismiss(sessionID, projects.NormalizePath(sess.Cwd))
				}
			}
			// Remove session from its project's sessions array.
			projectMgr.DismissSession(sessionID, sess.Slug)
			// Remove from store — broadcasts session-remove to all clients
			// (which the cleanup goroutine catches to drop meta), then
			// also drop meta synchronously to defeat any subscriber lag.
			sessions.Remove(sessionID)
			forgetMeta(sessionID)
			if subs != nil {
				subs.Unsubscribe(sessionID)
			}
			if fileMon != nil {
				fileMon.NotifySessionDied(sessionID)
			}
			writeJSON(w, map[string]any{"ok": true, "data": map[string]any{}})

		case "wait":
			handleWait(w, r, sessions, sessionID)

		default:
			http.NotFound(w, r)
		}
	})

	// ── WebSocket proxy ──

	wsProxy := wsproxy.New(func(sessionID string) (string, error) {
		sess, ok := sessions.Get(sessionID)
		if !ok {
			return "", fmt.Errorf("session %s not found", sessionID)
		}
		if sess.Peer != "" {
			// Remote session: return empty socket path. The WS handler
			// checks for this and uses the peer proxy path instead.
			return "", fmt.Errorf("session %s is remote (peer: %s)", sessionID, sess.Peer)
		}
		if sess.SocketPath == "" {
			return "", fmt.Errorf("session %s has no socket", sessionID)
		}
		return sess.SocketPath, nil
	}, sessions)

	// WS handler: local sessions use the Unix proxy, remote sessions
	// are proxied to the spoke's WS endpoint over TCP.
	mux.HandleFunc("/ws/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionID")

		// Check if this is a remote session.
		if peerManager != nil {
			if peer, originalID := peerManager.FindPeer(sessionID); peer != nil {
				peer.ProxyWS(w, r, originalID)
				return
			}
		}

		// Local session: use the existing Unix socket proxy.
		wsProxy.Handler()(w, r)
	})

	// ── Presence WebSocket ──

	mux.HandleFunc("/v1/presence", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			log.Printf("presence: accept: %v", err)
			return
		}

		clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
		client := &presence.Client{
			ID:          clientID,
			Conn:        conn,
			ConnectedAt: time.Now(),
		}

		// Read client-hello first.
		ctx := r.Context()
		_, data, err := conn.Read(ctx)
		if err != nil {
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
		var hello struct {
			Type                   string `json:"type"`
			DeviceType             string `json:"device_type"`
			NotificationPermission string `json:"notification_permission"`
		}
		if err := json.Unmarshal(data, &hello); err == nil && hello.Type == "client-hello" {
			client.DeviceType = hello.DeviceType
			client.NotificationPermission = hello.NotificationPermission
		}

		presenceTable.Add(client)
		log.Printf("presence: client %s connected (%s, notif=%s)", clientID, client.DeviceType, client.NotificationPermission)

		defer func() {
			presenceTable.Remove(clientID)
			conn.Close(websocket.StatusNormalClosure, "")
			log.Printf("presence: client %s disconnected", clientID)
		}()

		// Read state updates until disconnect.
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type              string  `json:"type"`
				Visibility        string  `json:"visibility"`
				Focused           bool    `json:"focused"`
				SelectedSessionID string  `json:"selected_session_id"`
				LastInteraction   float64 `json:"last_interaction"`
				Permission        string  `json:"permission"`
				ID                string  `json:"id"`
				Action            string  `json:"action"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "client-state":
				presenceTable.Update(clientID, presence.ClientState{
					Visibility:        msg.Visibility,
					Focused:           msg.Focused,
					SelectedSessionID: msg.SelectedSessionID,
					LastInteraction:   msg.LastInteraction,
				})
			case "notif-permission":
				presenceTable.SetPermission(clientID, msg.Permission)
			case "notif-ack":
				if notifRouter != nil {
					notifRouter.Ack(msg.ID)
				}
			}
		}
	})

	// ── SSE Events ──

	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// isOwned reports whether a session belongs to this node.
		// Local sessions (Peer=="") and devcontainer sessions (Local
		// peer) are owned; network peer sessions are not forwarded.
		isOwned := func(s *store.Session) bool {
			if s.Peer == "" {
				return true
			}
			return peerManager != nil && peerManager.IsLocalPeer(s.Peer)
		}

		// isOwnedEvent checks a store event. Session-upsert carries
		// the full session; remove/activity only have the ID, so we
		// extract the peer from the namespaced ID. Non-session events
		// (projects-update, peer-status) are always forwarded.
		isOwnedEvent := func(ev store.Event) bool {
			switch ev.Type {
			case "session-upsert":
				if ev.Session == nil {
					return true
				}
				return isOwned(ev.Session)
			case "session-remove", "session-activity":
				_, peerName := peering.ParseID(ev.ID)
				if peerName == "" {
					return true // local
				}
				return peerManager != nil && peerManager.IsLocalPeer(peerName)
			default:
				return true
			}
		}

		// Send current state as upserts (owned sessions only).
		for _, sess := range sessions.List() {
			s := sess
			if !isOwned(&s) {
				continue
			}
			sendSSE(w, "session-upsert", store.Event{
				Type:    "session-upsert",
				ID:      s.ID,
				Session: &s,
			})
		}
		flusher.Flush()

		// Stream updates (owned events only).
		ch, cancel := sessions.Subscribe()
		defer cancel()

		// Heartbeat: send an SSE comment every 30s to keep the connection
		// alive through idle periods. Without this, the hub's sseclient
		// idle timeout (60s) would fire on legitimately idle spokes, and
		// the browser's EventSource would have no way to detect a dead
		// hub connection.
		heartbeat := time.NewTicker(30 * time.Second)
		defer heartbeat.Stop()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case <-heartbeat.C:
				// SSE comment line: resets the client's idle timer
				// without producing a client-side event.
				fmt.Fprint(w, ":\n\n")
				flusher.Flush()
			case ev, open := <-ch:
				if !open {
					return
				}
				if !isOwnedEvent(ev) {
					continue
				}
				sendSSE(w, ev.Type, ev)
				flusher.Flush()
			}
		}
	})

	// ── Embedded frontend (SPA fallback) ──

	mux.Handle("/", spaHandler())

	// ── Load config ──

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	if err := cfg.ResolveTokens(); err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	remoteMode = cfg.Remote.Mode
	if remoteMode == "" {
		switch {
		case cfg.Tailscale.Enabled && cfg.Relay.Enabled:
			remoteMode = "tsnet, relay"
		case cfg.Tailscale.Enabled:
			remoteMode = "tsnet"
		case cfg.Relay.Enabled:
			remoteMode = "relay"
		}
	}
	if cfg.Relay.Enabled {
		relayStatus = map[string]string{"url": cfg.Relay.URL}
		if cfg.Remote.PublicURL != "" {
			relayStatus["public_url"] = cfg.Remote.PublicURL
		}
	}

	// ── Resolve TCP listen address and auth token ──

	resolved, err := cfg.ListenAddr()
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	tcpAddr = resolved

	tok, err := authtoken.LoadOrCreate(stateDir)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	authToken = tok

	// ── Replace any existing daemon via Unix socket ──

	sock := paths.SocketPath()
	if err := unixipc.Replace(sock); err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: %v\n", err)
		return 1
	}

	// ── Shutdown endpoint (Unix socket only) ──
	// The netauth middleware blocks this on TCP.
	// Tailscale also blocks it (peer identity, not localhost).

	var sockSrv *http.Server
	var tcpSrv *http.Server

	// shutdownCh is closed by the /v1/shutdown handler to trigger the
	// same graceful exit path as SIGINT/SIGTERM. Without this, the
	// handler only shut down HTTP listeners, leaving background
	// goroutines (peering, discovery, file monitors) running
	// indefinitely as a zombie process.
	shutdownCh := make(chan struct{})
	var shutdownOnce sync.Once

	mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		shutdownOnce.Do(func() {
			log.Printf("shutdown requested — exiting")
			close(shutdownCh)
		})
	})

	// ── Unix socket listener (local IPC, no auth) ──

	sockLn, err := unixipc.Listen(sock)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	sockSrv = &http.Server{Handler: mux}
	go func() {
		if err := sockSrv.Serve(sockLn); err != http.ErrServerClosed {
			log.Printf("unix socket listener: %v", err)
		}
	}()
	log.Printf("unix socket: %s", sock)

	// ── TCP listener (always, token-authenticated) ──

	authedHandler := netauth.Middleware(authToken, mux)
	tcpSrv = &http.Server{Addr: tcpAddr, Handler: authedHandler}

	tcpLn, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		log.Fatalf("FATAL: tcp listener on %s: %v", tcpAddr, err)
	}

	log.Printf("tcp listener on %s (token-authenticated)", tcpAddr)
	go func() {
		if err := tcpSrv.Serve(tcpLn); err != http.ErrServerClosed {
			log.Printf("tcp listener: %v", err)
		}
	}()

	// ── Optional outbound relay client ──

	var relayCancel context.CancelFunc
	if cfg.Relay.Enabled {
		relayCtx, cancel := context.WithCancel(context.Background())
		relayCancel = cancel
		go relayclient.Run(relayCtx, relayclient.Config{
			URL:      cfg.Relay.URL,
			Token:    cfg.Relay.Token,
			LocalURL: "http://" + tcpAddr,
		})
		log.Printf("relay: enabled (%s)", cfg.Relay.URL)
	}

	// ── Sleep detection ──

	sleepWatcher := sleep.NewWatcher()
	defer sleepWatcher.Stop()

	// ── Peer connections (hub protocol) ──

	hostname, _ := os.Hostname()
	if len(cfg.Peers) > 0 || cfg.Discovery.Devcontainers || (cfg.Tailscale.Enabled && cfg.Discovery.Tailscale) {
		peerManager = peering.NewManager(cfg.Peers, sessions, hostname)
		peerManager.Start()
		if len(cfg.Peers) > 0 {
			log.Printf("peering: %d peer(s) configured", len(cfg.Peers))
		}

		// Reconnect all peers after system sleep.
		go func() {
			for range sleepWatcher.C() {
				peerManager.OnSleep()
			}
		}()
	}

	// ── Devcontainer discovery ──

	var dcWatcher *devcontainers.Watcher
	if cfg.Discovery.Devcontainers {
		dcWatcher = devcontainers.NewWatcher(peerManager)
		if dcWatcher != nil {
			dcWatcher.Start()
			log.Printf("devcontainers: discovery enabled")
		} else {
			log.Printf("devcontainers: docker CLI not found, skipping discovery")
		}
	}

	// Signal channel: declared here so the tailscale discovery goroutine
	// can select on it to avoid blocking shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// ── Optional tailscale listener ──

	if cfg.Tailscale.Enabled {
		tsListener = tsauth.Start(tsauth.Config{
			Hostname: cfg.Tailscale.Hostname,
			Allow:    cfg.Tailscale.Allow,
			AuthKey:  cfg.Tailscale.AuthKey,
		}, stateDir, mux)
		defer tsListener.Shutdown()

		// Start tailscale peer discovery once the listener is ready.
		if cfg.Discovery.Tailscale && peerManager != nil {
			tsDiscovery = tsdiscovery.New(tsdiscovery.Config{
				Manager:        peerManager,
				StateDir:       stateDir,
				ManualPeerURLs: tsdiscovery.ManualPeerURLs(cfg.Peers),
				HostnamePrefix: cfg.Tailscale.Hostname,
			})
			tsDiscoveryCtx, tsDiscoveryCancel := context.WithCancel(context.Background())
			defer tsDiscoveryCancel()
			go func() {
				select {
				case <-tsListener.Ready():
				case <-tsDiscoveryCtx.Done():
					return
				}
				tsDiscovery.SetTailscale(
					tsListener.LocalClient(),
					tsListener.Transport(),
					tsListener.FQDN(),
				)
				tsDiscovery.Start()
			}()
		}
	}

	// ── Signal handling for graceful shutdown ──

	log.Printf("jumpd %s ready", version)

	select {
	case sig := <-sigCh:
		log.Printf("received %v — shutting down", sig)
	case <-shutdownCh:
		log.Printf("shutdown requested — shutting down")
	}

	if relayCancel != nil {
		relayCancel()
	}
	if tsDiscovery != nil {
		tsDiscovery.Stop()
	}
	if dcWatcher != nil {
		dcWatcher.Stop()
	}
	if peerManager != nil {
		peerManager.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tcpSrv.Shutdown(ctx)
	sockSrv.Shutdown(ctx)
	unixipc.Cleanup(sock)

	log.Printf("jumpd stopped")
	return 0
}

type relayHealthStatus struct {
	URL       string `json:"url"`
	PublicURL string `json:"public_url,omitempty"`
}

// runStatus queries the running daemon via Unix socket and prints health info.
func runStatus(stdout, stderr io.Writer) int {
	sock := paths.SocketPath()
	client := unixipc.Client(sock)

	resp, err := client.Get("http://localhost/v1/health")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: not running (socket: %s)\n", sock)
		return 1
	}
	defer resp.Body.Close()

	var health struct {
		OK   bool `json:"ok"`
		Data struct {
			Version         string             `json:"version"`
			Status          string             `json:"status"`
			Listen          string             `json:"listen"`
			RemoteMode      string             `json:"remote_mode,omitempty"`
			TailscaleURL    string             `json:"tailscale_url,omitempty"`
			Tailscale       *tsHealth          `json:"tailscale,omitempty"`
			Relay           *relayHealthStatus `json:"relay,omitempty"`
			UpdateAvailable string             `json:"update_available,omitempty"`
			Sessions        *struct {
				LocalAlive  int `json:"local_alive"`
				RemoteAlive int `json:"remote_alive"`
				Dead        int `json:"dead"`
			} `json:"sessions,omitempty"`
			Peers []struct {
				Name         string `json:"name"`
				URL          string `json:"url"`
				Status       string `json:"status"`
				SessionCount int    `json:"session_count"`
				LastError    string `json:"last_error,omitempty"`
			} `json:"peers,omitempty"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil || !health.OK {
		_, _ = fmt.Fprintf(stderr, "jumpd: unexpected health response\n")
		return 1
	}

	d := health.Data
	_, _ = fmt.Fprintf(stdout, "jumpd %s (%s)\n", d.Version, d.Status)
	_, _ = fmt.Fprintf(stdout, "  tcp:    %s\n", d.Listen)
	_, _ = fmt.Fprintf(stdout, "  socket: %s\n", sock)
	printRemoteStatus(stdout, d.RemoteMode, d.TailscaleURL, d.Tailscale, d.Relay)
	if d.UpdateAvailable != "" {
		_, _ = fmt.Fprintf(stdout, "  update: %s available\n", d.UpdateAvailable)
	}

	// Sessions.
	if s := d.Sessions; s != nil {
		total := s.LocalAlive + s.RemoteAlive + s.Dead
		_, _ = fmt.Fprintf(stdout, "\nSessions: %d alive", s.LocalAlive+s.RemoteAlive)
		if s.RemoteAlive > 0 {
			_, _ = fmt.Fprintf(stdout, " (%d local, %d remote)", s.LocalAlive, s.RemoteAlive)
		}
		_, _ = fmt.Fprintf(stdout, ", %d dead (%d total)\n", s.Dead, total)
	}

	// Peers.
	if len(d.Peers) > 0 {
		_, _ = fmt.Fprintf(stdout, "\nPeers:\n")
		for _, p := range d.Peers {
			var detail string
			switch p.Status {
			case "connected":
				detail = fmt.Sprintf("%d session%s", p.SessionCount, plural(p.SessionCount))
			case "connecting":
				detail = "connecting..."
			case "offline":
				detail = "offline"
			default:
				if p.LastError != "" {
					detail = p.LastError
				} else {
					detail = "disconnected"
				}
			}
			_, _ = fmt.Fprintf(stdout, "  %s %s (%s)\n", statusDot(p.Status), p.Name, detail)
			_, _ = fmt.Fprintf(stdout, "    %s\n", p.URL)
		}
	}

	return 0
}

func printRemoteStatus(stdout io.Writer, mode, tailscaleURL string, ts *tsHealth, relay *relayHealthStatus) {
	if mode == "" && tailscaleURL == "" && ts == nil && relay == nil {
		return
	}
	if mode != "" {
		label := "remote mode"
		if strings.Contains(mode, ",") {
			label = "remote modes"
		}
		_, _ = fmt.Fprintf(stdout, "  %s: %s\n", label, mode)
	}
	if ts != nil {
		switch {
		case ts.AuthURL != "":
			_, _ = fmt.Fprintf(stdout, "  tsnet: needs login\n")
			_, _ = fmt.Fprintf(stdout, "    login: %s\n", ts.AuthURL)
		case ts.Connected:
			url := tailscaleURL
			if url == "" && ts.FQDN != "" {
				url = "https://" + ts.FQDN
			}
			if url != "" {
				_, _ = fmt.Fprintf(stdout, "  tsnet: connected %s\n", url)
			} else {
				_, _ = fmt.Fprintf(stdout, "  tsnet: connected\n")
			}
		default:
			_, _ = fmt.Fprintf(stdout, "  tsnet: connecting\n")
		}
	} else if tailscaleURL != "" {
		_, _ = fmt.Fprintf(stdout, "  tsnet: %s\n", tailscaleURL)
	}
	if relay != nil {
		if relay.PublicURL != "" {
			_, _ = fmt.Fprintf(stdout, "  relayd: %s\n", relay.PublicURL)
		}
		if relay.URL != "" {
			_, _ = fmt.Fprintf(stdout, "  relay agent: %s\n", relay.URL)
		}
	}
}

// appendOfflinePeers merges active peers from the manager with offline
// discovered peers from tailscale discovery. Returns nil if no peers.
func appendOfflinePeers(mgr *peering.Manager, disc *tsdiscovery.Watcher) []peering.PeerInfo {
	var peers []peering.PeerInfo
	if mgr != nil && mgr.HasPeers() {
		peers = mgr.PeerStatus()
	}
	if disc != nil {
		for _, op := range disc.OfflinePeers() {
			peers = append(peers, peering.PeerInfo{
				Name:   op.Name,
				URL:    "https://" + op.FQDN,
				Status: "offline",
			})
		}
	}
	return peers
}

func statusDot(status string) string {
	switch status {
	case "connected":
		return "\u2022" // bullet
	case "connecting":
		return "\u25cb" // open circle
	case "offline":
		return "\u25cb" // open circle
	default:
		return "\u2717" // X mark
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// runAuth queries the running daemon for the TCP address and auth token.
func runAuth(stdout, stderr io.Writer) int {
	sock := paths.SocketPath()
	client := unixipc.Client(sock)

	resp, err := client.Get("http://localhost/v1/health")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jumpd: not running (socket: %s)\n", sock)
		return 1
	}
	defer resp.Body.Close()

	var health struct {
		OK   bool `json:"ok"`
		Data struct {
			Listen    string `json:"listen"`
			AuthToken string `json:"auth_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil || !health.OK {
		_, _ = fmt.Fprintf(stderr, "jumpd: unexpected health response\n")
		return 1
	}

	if health.Data.AuthToken == "" {
		_, _ = fmt.Fprintf(stderr, "jumpd: could not retrieve auth token\n")
		return 1
	}

	url := fmt.Sprintf("http://%s/auth/login?token=%s", health.Data.Listen, health.Data.AuthToken)

	_, _ = fmt.Fprintf(stdout, "Listen:     %s\n", health.Data.Listen)
	_, _ = fmt.Fprintf(stdout, "Auth token: %s\n", health.Data.AuthToken)
	_, _ = fmt.Fprintf(stdout, "\nOpen this URL to authenticate:\n  %s\n", url)

	return 0
}

// buildSessionInfos converts store sessions to project SessionInfo structs.
func buildSessionInfos(sessions *store.Store) []projects.SessionInfo {
	list := sessions.List()
	infos := make([]projects.SessionInfo, len(list))
	for i, s := range list {
		infos[i] = projects.SessionInfo{
			ID:            s.ID,
			Cwd:           s.Cwd,
			WorkspaceRoot: s.WorkspaceRoot,
			Remotes:       s.Remotes,
			Host:          s.Peer,
			Alive:         s.Alive,
			Slug:          s.Slug,
		}
	}
	return infos
}

func sendSSE(w http.ResponseWriter, event string, payload any) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, bytes)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message},
	})
}
