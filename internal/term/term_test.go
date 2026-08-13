package term

import (
	"strings"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestManagerDisabled(t *testing.T) {
	m := NewManager(nil, 2, time.Minute)
	if m.Enabled() {
		t.Fatal("nil launcher must mean disabled")
	}
	if _, err := m.Create(80, 24); err == nil {
		t.Fatal("Create should fail when disabled")
	}
}

func TestMockSessionLifecycle(t *testing.T) {
	m := NewManager(NewMockLauncher(), 2, time.Minute)
	v, err := m.Create(80, 24)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s, ok := m.Get(v.ID)
	if !ok {
		t.Fatal("session not found after create")
	}

	// The banner lands in the replay buffer.
	waitFor(t, func() bool {
		replay, _, cancel := s.Subscribe()
		cancel()
		return strings.Contains(string(replay), "mock shell")
	}, "banner in replay")

	// Typed input produces command output on the live stream.
	_, ch, cancel := s.Subscribe()
	defer cancel()
	if err := s.Input([]byte("whoami\r")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	var out strings.Builder
	deadline := time.After(2 * time.Second)
	for !strings.Contains(out.String(), "mock") {
		select {
		case c, open := <-ch:
			if !open {
				t.Fatalf("stream closed early; got %q", out.String())
			}
			out.Write(c.Data)
		case <-deadline:
			t.Fatalf("no whoami output; got %q", out.String())
		}
	}

	if !m.Close(v.ID) {
		t.Fatal("Close should report the session existed")
	}
	waitFor(t, func() bool { return len(m.List()) == 0 }, "session removal")
	if err := s.Input([]byte("x")); err == nil {
		t.Error("Input after close should fail")
	}
}

func TestSessionLimit(t *testing.T) {
	m := NewManager(NewMockLauncher(), 1, time.Minute)
	v, err := m.Create(80, 24)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Create(80, 24); err == nil {
		t.Error("second session should exceed the limit")
	}
	m.Close(v.ID)
	waitFor(t, func() bool {
		_, err := m.Create(80, 24)
		return err == nil
	}, "slot to free after close")
}

func TestExitCommandEndsSession(t *testing.T) {
	m := NewManager(NewMockLauncher(), 2, time.Minute)
	v, _ := m.Create(80, 24)
	s, _ := m.Get(v.ID)
	_, ch, cancel := s.Subscribe()
	defer cancel()
	_ = s.Input([]byte("exit\r"))
	deadline := time.After(2 * time.Second)
	for {
		select {
		case c, open := <-ch:
			if !open || c.Exit {
				return // stream ended as expected
			}
		case <-deadline:
			t.Fatal("exit did not end the stream")
		}
	}
}
