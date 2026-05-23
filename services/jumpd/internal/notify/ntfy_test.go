package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sting8k/jump/services/jumpd/internal/store"
)

func TestWorkspaceLabelUsesWorkspaceBasename(t *testing.T) {
	got := workspaceLabel(store.Session{WorkspaceRoot: "/Users/bean/Documents/Develope/pi-agent-ext/gmux", Cwd: "/tmp"})
	if got != "gmux" {
		t.Fatalf("workspace = %q, want gmux", got)
	}
}

func TestFormatNtfyMessagePrivacySafe(t *testing.T) {
	msg := formatNtfyMessage(&pendingNotif{workspace: "gmux", notifType: "finished", title: "secret-title", body: "Finished (2m 0s)"}, false)
	if msg != "[gmux] session finished" {
		t.Fatalf("message = %q", msg)
	}
}

func TestFormatNtfyMessageWithDetails(t *testing.T) {
	msg := formatNtfyMessage(&pendingNotif{workspace: "gmux", notifType: "unread", title: "api", body: "New output"}, true)
	if msg != "[gmux] api: new output" {
		t.Fatalf("message = %q", msg)
	}
}

func TestFormatCoalescedNtfyMessageGroupsWorkspaces(t *testing.T) {
	msg := formatCoalescedNtfyMessage([]*pendingNotif{
		{workspace: "gmux"},
		{workspace: "gmux"},
		{workspace: "agent"},
	})
	want := "[agent] 1 session needs attention\n[gmux] 2 sessions need attention"
	if msg != want {
		t.Fatalf("message = %q, want %q", msg, want)
	}
}

func TestSendNtfyPostsTopicAndHeaders(t *testing.T) {
	var gotPath, gotAuth, gotTitle, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotTitle = r.Header.Get("Title")
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := sendNtfy(context.Background(), srv.Client(), NtfyConfig{
		ServerURL: srv.URL,
		TopicID:   "jump-topic",
		Token:     "tk_secret",
	}, "[gmux] session finished")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/jump-topic" {
		t.Fatalf("path = %q, want /jump-topic", gotPath)
	}
	if gotAuth != "Bearer tk_secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotTitle != "jump" {
		t.Fatalf("title = %q", gotTitle)
	}
	if gotBody != "[gmux] session finished" {
		t.Fatalf("body = %q", gotBody)
	}
}
