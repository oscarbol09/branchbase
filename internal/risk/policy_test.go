package risk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("default policy failed validation: %v", err)
	}

	if p.ConfidenceThreshold != 0.75 {
		t.Errorf("expected 0.75 confidence threshold, got %f", p.ConfidenceThreshold)
	}
}

func TestPolicyPathExclusion(t *testing.T) {
	p := DefaultPolicy()

	if !p.IsPathExcluded("migrations/seeds/01_users.sql") {
		t.Errorf("expected migrations/seeds/01_users.sql to be excluded")
	}

	if !p.IsPathExcluded("migrations/test_data/02_orders.sql") {
		t.Errorf("expected migrations/test_data/02_orders.sql to be excluded")
	}

	if p.IsPathExcluded("migrations/001_create_users.sql") {
		t.Errorf("did not expect standard migration to be excluded")
	}
}

func TestPolicyMatchesAlwaysCritical(t *testing.T) {
	p := DefaultPolicy()

	match, pattern := p.MatchesAlwaysCritical("ALTER TABLE users DROP COLUMN email; DROP TABLE legacy_orders;")
	if !match || pattern != "DROP TABLE" {
		t.Errorf("expected DROP TABLE match, got match=%v pattern=%s", match, pattern)
	}

	match, _ = p.MatchesAlwaysCritical("CREATE INDEX idx_users_email ON users(email);")
	if match {
		t.Errorf("did not expect match for CREATE INDEX")
	}

	// Procedural block with dollar quotes containing DROP TABLE
	proceduralSQL := "DO $$ BEGIN DROP TABLE legacy_orders; END $$;"
	match, pattern = p.MatchesAlwaysCritical(proceduralSQL)
	if !match || pattern != "DROP TABLE" {
		t.Errorf("expected DROP TABLE match inside DO $$ block, got match=%v pattern=%s", match, pattern)
	}
}

func TestPolicyEvaluate(t *testing.T) {
	p := DefaultPolicy()

	// High confidence LOW risk -> Allow
	rLow := &MigrationRisk{Level: RiskLow, Confidence: 0.95}
	dLow := p.Evaluate(rLow)
	if dLow.Action != ActionAllow || dLow.EffectiveLevel != RiskLow {
		t.Errorf("expected Allow for high confidence LOW, got %v", dLow)
	}

	// Low confidence LOW risk -> Escalate to MEDIUM -> Warn
	rLowUncertain := &MigrationRisk{Level: RiskLow, Confidence: 0.60}
	dLowUncertain := p.Evaluate(rLowUncertain)
	if dLowUncertain.EffectiveLevel != RiskMedium || dLowUncertain.Action != ActionWarn {
		t.Errorf("expected escalated MEDIUM (Warn), got %v", dLowUncertain)
	}

	// High risk -> Confirm
	rHigh := &MigrationRisk{Level: RiskHigh, Confidence: 0.90}
	dHigh := p.Evaluate(rHigh)
	if dHigh.Action != ActionConfirm || !dHigh.RequiresConfirmation {
		t.Errorf("expected Confirm for HIGH risk, got %v", dHigh)
	}

	// Critical risk -> Block
	rCrit := &MigrationRisk{Level: RiskCritical, Confidence: 0.99}
	dCrit := p.Evaluate(rCrit)
	if dCrit.Action != ActionBlock || !dCrit.Blocked {
		t.Errorf("expected Block for CRITICAL risk, got %v", dCrit)
	}
}

func TestLoadPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	policyDir := filepath.Join(tmpDir, ".branchbase")
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		t.Fatal(err)
	}

	customYAML := `
version: 1
on_risk:
  LOW: allow
  MEDIUM: allow
  HIGH: warn
  CRITICAL: block
confidence_threshold: 0.85
on_jev_unavailable: block
always_critical:
  - "DROP DATABASE"
`
	if err := os.WriteFile(filepath.Join(policyDir, "risk-policy.yml"), []byte(customYAML), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicy(tmpDir, "")
	if err != nil {
		t.Fatalf("failed to load custom policy: %v", err)
	}

	if p.ConfidenceThreshold != 0.85 {
		t.Errorf("expected 0.85 threshold, got %f", p.ConfidenceThreshold)
	}
	if p.OnJevUnavailable != "block" {
		t.Errorf("expected on_jev_unavailable block, got %s", p.OnJevUnavailable)
	}
}

func TestPolicyPathExclusionDirectoryBoundaries(t *testing.T) {
	policy := &RiskPolicy{ExcludePaths: []string{filepath.Join("migrations", "seeds", "**")}}
	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{"directory", filepath.Join("migrations", "seeds"), true},
		{"direct child", filepath.Join("migrations", "seeds", "001.sql"), true},
		{"nested child", filepath.Join("migrations", "seeds", "users", "001.sql"), true},
		{"sibling directory", filepath.Join("migrations", "seeds_archive", "001_initial.sql"), false},
		{"sibling suffix", filepath.Join("migrations", "seeds2", "001.sql"), false},
		{"sibling file", filepath.Join("migrations", "seeds.sql"), false},
		{"parent directory", "migrations", false},
		{"unrelated", filepath.Join("other", "seeds", "001.sql"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.IsPathExcluded(tt.path); got != tt.want {
				t.Errorf("IsPathExcluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPolicyPathExclusionOrdinaryGlob(t *testing.T) {
	policy := &RiskPolicy{ExcludePaths: []string{filepath.Join("migrations", "*.seed.sql")}}
	if !policy.IsPathExcluded(filepath.Join("migrations", "users.seed.sql")) {
		t.Error("ordinary filename glob should still match")
	}
	if policy.IsPathExcluded(filepath.Join("migrations", "users.sql")) {
		t.Error("unmatched filename should not be excluded")
	}
}
