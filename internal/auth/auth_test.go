package auth

import (
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Error("correct password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("wrong password accepted")
	}
	if VerifyPassword(hash, "") {
		t.Error("empty password accepted")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	for _, bad := range []string{
		"", "plaintext", "$argon2id$v=19$m=65536", "$md5$whatever$x$y",
		"$argon2id$v=19$m=65536,t=3,p=2$!!notbase64!!$AAAA",
	} {
		if VerifyPassword(bad, "anything") {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestSessions(t *testing.T) {
	s := NewSessions(time.Hour, t.TempDir())
	token, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(token) {
		t.Error("fresh token invalid")
	}
	if s.Valid("forged-token") {
		t.Error("forged token accepted")
	}
	s.Revoke(token)
	if s.Valid(token) {
		t.Error("revoked token still valid")
	}
}

func TestSessionsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s1 := NewSessions(time.Hour, dir)
	token, _ := s1.Create()

	s2 := NewSessions(time.Hour, dir)
	if !s2.Valid(token) {
		t.Error("session lost across restart")
	}
}

func TestSessionExpiry(t *testing.T) {
	s := NewSessions(-time.Minute, t.TempDir()) // already expired
	token, _ := s.Create()
	if s.Valid(token) {
		t.Error("expired session accepted")
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)
	ip := "10.0.0.5"
	for i := 0; i < 3; i++ {
		if !l.Allow(ip) {
			t.Fatalf("attempt %d should be allowed", i)
		}
		l.Fail(ip)
	}
	if l.Allow(ip) {
		t.Error("4th attempt should be blocked")
	}
	if !l.Allow("10.0.0.6") {
		t.Error("other IPs must be unaffected")
	}
	l.Reset(ip)
	if !l.Allow(ip) {
		t.Error("reset should clear failures")
	}
}
