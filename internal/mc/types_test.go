package mc

import (
	"strings"
	"testing"
)

func validSpec() CreateSpec {
	return CreateSpec{ID: "survival", Flavor: "paper", Version: "1.21.4", Port: 25565}
}

func TestCreateSpecValidates(t *testing.T) {
	s := validSpec()
	if err := s.Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestCreateSpecRejectsMultilineMOTD(t *testing.T) {
	// The MOTD lands verbatim in server.properties: a newline would inject
	// arbitrary property lines.
	for _, motd := range []string{"hi\nenable-rcon=true", "hi\rrcon.password=x"} {
		s := validSpec()
		s.MOTD = motd
		if err := s.Validate(); err == nil {
			t.Errorf("MOTD %q should be rejected", motd)
		}
	}
	long := validSpec()
	long.MOTD = strings.Repeat("x", 300)
	if err := long.Validate(); err == nil {
		t.Error("over-long MOTD should be rejected")
	}
}
