package services

import "testing"

func TestCheckActionAllowlist(t *testing.T) {
	allow := []string{"plexmediaserver.service", "smbd.service"}

	if err := CheckAction(allow, "smbd.service", "restart"); err != nil {
		t.Errorf("allowlisted unit rejected: %v", err)
	}
	if err := CheckAction(allow, "sshd.service", "stop"); err == nil {
		t.Error("non-allowlisted unit accepted")
	}
	if err := CheckAction(allow, "smbd.service", "mask"); err == nil {
		t.Error("invalid verb accepted")
	}
	if err := CheckAction(allow, "smbd.service; rm -rf /", "stop"); err == nil {
		t.Error("injection-shaped unit name accepted")
	}
	if err := CheckAction(allow, "../evil.service", "stop"); err == nil {
		t.Error("path-traversal unit name accepted")
	}
}

func TestCheckUnitNameShape(t *testing.T) {
	// Even if somehow allowlisted, malformed names must fail the regex.
	bad := "bad name.service"
	if err := CheckUnit([]string{bad}, bad); err == nil {
		t.Error("unit with space accepted")
	}
}
