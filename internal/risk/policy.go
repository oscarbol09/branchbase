package risk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PolicyDecision represents the evaluated action for a migration assessment.
type PolicyDecision struct {
	Action      PolicyAction `json:"action"`
	EffectiveLevel RiskLevel `json:"effective_level"`
	Reason      string       `json:"reason"`
	Blocked     bool         `json:"blocked"`
	RequiresConfirmation bool `json:"requires_confirmation"`
}

// RiskPolicy holds the rules from .branchbase/risk-policy.yml.
type RiskPolicy struct {
	Version             int                     `yaml:"version"`
	OnRisk              map[string]string       `yaml:"on_risk"`
	ConfidenceThreshold float64                 `yaml:"confidence_threshold"`
	OnJevUnavailable    string                  `yaml:"on_jev_unavailable"`
	AlwaysCritical      []string                `yaml:"always_critical"`
	ExcludePaths        []string                `yaml:"exclude_paths"`
	ReportFormat        string                  `yaml:"report_format"`
	SaveReport          string                  `yaml:"save_report"`
}

// DefaultPolicy returns safe, standard defaults for the risk policy.
func DefaultPolicy() *RiskPolicy {
	return &RiskPolicy{
		Version: 1,
		OnRisk: map[string]string{
			string(RiskLow):      string(ActionAllow),
			string(RiskMedium):   string(ActionWarn),
			string(RiskHigh):     string(ActionConfirm),
			string(RiskCritical): string(ActionBlock),
		},
		ConfidenceThreshold: 0.75,
		OnJevUnavailable:    string(ActionWarn),
		AlwaysCritical: []string{
			"DROP TABLE",
			"TRUNCATE",
			"DROP DATABASE",
			"DROP SCHEMA",
		},
		ExcludePaths: []string{
			"migrations/seeds/**",
			"migrations/test_data/**",
		},
		ReportFormat: "pretty",
		SaveReport:   ".branchbase/last-risk-report.json",
	}
}

// LoadPolicy attempts to read .branchbase/risk-policy.yml from repoRoot or custom path.
// If the file does not exist, it returns the DefaultPolicy without error.
func LoadPolicy(repoRoot, customPath string) (*RiskPolicy, error) {
	targetPath := customPath
	if targetPath == "" {
		targetPath = filepath.Join(repoRoot, ".branchbase", "risk-policy.yml")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicy(), nil
		}
		return nil, fmt.Errorf("failed to read risk policy at %s: %w", targetPath, err)
	}

	policy := DefaultPolicy()
	if err := yaml.Unmarshal(data, policy); err != nil {
		return nil, fmt.Errorf("invalid YAML in risk policy %s: %w", targetPath, err)
	}

	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid risk policy configuration: %w", err)
	}

	return policy, nil
}

// Validate ensures all actions and thresholds in the policy are valid.
func (p *RiskPolicy) Validate() error {
	if p.ConfidenceThreshold < 0 || p.ConfidenceThreshold > 1.0 {
		return fmt.Errorf("confidence_threshold must be between 0.0 and 1.0, got %f", p.ConfidenceThreshold)
	}

	validActions := map[string]bool{
		string(ActionAllow):   true,
		string(ActionWarn):    true,
		string(ActionConfirm): true,
		string(ActionBlock):   true,
	}

	for lvl, act := range p.OnRisk {
		if !validActions[strings.ToLower(act)] {
			return fmt.Errorf("invalid action %q for risk level %q", act, lvl)
		}
	}

	if p.OnJevUnavailable != "" && !validActions[strings.ToLower(p.OnJevUnavailable)] {
		return fmt.Errorf("invalid on_jev_unavailable action %q", p.OnJevUnavailable)
	}

	return nil
}

// IsPathExcluded checks if a file path matches any exclusion glob patterns.
func (p *RiskPolicy) IsPathExcluded(filePath string) bool {
	cleanPath := filepath.ToSlash(filePath)
	for _, pattern := range p.ExcludePaths {
		cleanPattern := filepath.ToSlash(pattern)
		// A trailing ** matches the directory and descendants, not sibling names.
		if strings.HasSuffix(cleanPattern, "/**") {
			prefix := strings.TrimSuffix(cleanPattern, "/**")
			if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/") {
				return true
			}
		}
		if matched, _ := filepath.Match(cleanPattern, cleanPath); matched {
			return true
		}
	}
	return false
}

// MatchesAlwaysCritical checks executable SQL tokens for configured critical patterns using Postgres dialect.
func (p *RiskPolicy) MatchesAlwaysCritical(sql string) (bool, string) {
	return p.MatchesAlwaysCriticalForEngine(sql, "postgres")
}

// MatchesAlwaysCriticalForEngine checks executable SQL tokens for configured critical patterns for a specific engine.
func (p *RiskPolicy) MatchesAlwaysCriticalForEngine(sql, engine string) (bool, string) {
	upperSQL := strings.ToUpper(sqlForPolicyMatchingForEngine(sql, engine))
	for _, pattern := range p.AlwaysCritical {
		if normalized := strings.ToUpper(sqlForPolicyMatchingForEngine(pattern, engine)); normalized != "" && strings.Contains(upperSQL, normalized) {
			return true, pattern
		}
	}
	return false, ""
}

func sqlForPolicyMatchingForEngine(sql string, engine string) string {
	isMySQL := strings.EqualFold(engine, "mysql") || strings.EqualFold(engine, "mariadb")
	isPostgres := strings.EqualFold(engine, "postgres") || strings.EqualFold(engine, "postgresql") || engine == ""

	var out strings.Builder
	writeSpace := func() {
		if out.Len() > 0 {
			current := out.String()
			if current[len(current)-1] != ' ' {
				out.WriteByte(' ')
			}
		}
	}

	for i := 0; i < len(sql); {
		switch {
		case sql[i] == '#' || (i+1 < len(sql) && sql[i:i+2] == "--"):
			if sql[i] == '#' {
				i++
			} else {
				i += 2
			}
			for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
				i++
			}
			writeSpace()

		case isMySQL && i+2 < len(sql) && sql[i:i+3] == "/*!":
			// MySQL executable comment
			i += 3
			for i < len(sql) && sql[i] >= '0' && sql[i] <= '9' {
				i++
			}
			writeSpace()

		case i+1 < len(sql) && sql[i:i+2] == "/*":
			if isPostgres {
				depth := 1
				i += 2
				for i < len(sql) && depth > 0 {
					if i+1 < len(sql) && sql[i:i+2] == "/*" {
						depth++
						i += 2
						continue
					}
					if i+1 < len(sql) && sql[i:i+2] == "*/" {
						depth--
						i += 2
						continue
					}
					i++
				}
			} else {
				i += 2
				for i < len(sql) {
					if i+1 < len(sql) && sql[i:i+2] == "*/" {
						i += 2
						break
					}
					i++
				}
			}
			writeSpace()

		case isMySQL && i+1 < len(sql) && sql[i:i+2] == "*/":
			i += 2
			writeSpace()

		case sql[i] == '$':
			end := i + 1
			for end < len(sql) && (sql[end] == '_' || (sql[end] >= 'a' && sql[end] <= 'z') || (sql[end] >= 'A' && sql[end] <= 'Z') || (sql[end] >= '0' && sql[end] <= '9')) {
				end++
			}
			if end < len(sql) && sql[end] == '$' {
				tag := sql[i : end+1]
				if closeAt := strings.Index(sql[end+1:], tag); closeAt >= 0 {
					body := sql[end+1 : end+1+closeAt]
					// Recursively normalize procedural dollar-quote bodies so DROP TABLE, TRUNCATE, etc.
					// inside DO $$ or CREATE FUNCTION blocks are preserved for risk policy evaluation.
					normBody := sqlForPolicyMatchingForEngine(body, engine)
					if normBody != "" {
						out.WriteString(" ")
						out.WriteString(normBody)
						out.WriteString(" ")
					}
					i = end + 1 + closeAt + len(tag)
					writeSpace()
					continue
				}
			}
			out.WriteByte(sql[i])
			i++

		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			quote := sql[i]
			i++
			for i < len(sql) {
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				if sql[i] == '\\' && quote == '\'' && i+1 < len(sql) {
					i += 2
					continue
				}
				i++
			}
			writeSpace()

		default:
			out.WriteByte(sql[i])
			i++
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

// Evaluate determines the policy action for a given migration risk assessment.
func (p *RiskPolicy) Evaluate(risk *MigrationRisk) PolicyDecision {
	effectiveLevel := risk.Level
	reason := fmt.Sprintf("Evaluated level %s", risk.Level)

	// Step 1: Check confidence threshold escalation
	if risk.Confidence > 0 && risk.Confidence < p.ConfidenceThreshold {
		oldLevel := effectiveLevel
		effectiveLevel = effectiveLevel.Escalate()
		reason = fmt.Sprintf("Confidence (%.0f%%) below threshold (%.0f%%); escalated %s -> %s",
			risk.Confidence*100, p.ConfidenceThreshold*100, oldLevel, effectiveLevel)
	}

	// Step 2: Resolve configured action for effective level
	actionStr, exists := p.OnRisk[string(effectiveLevel)]
	if !exists {
		// Fallback defaults
		switch effectiveLevel {
		case RiskLow:
			actionStr = string(ActionAllow)
		case RiskMedium:
			actionStr = string(ActionWarn)
		case RiskHigh:
			actionStr = string(ActionConfirm)
		case RiskCritical:
			actionStr = string(ActionBlock)
		default:
			actionStr = string(ActionWarn)
		}
	}

	action, _ := ParsePolicyAction(actionStr)

	return PolicyDecision{
		Action:               action,
		EffectiveLevel:       effectiveLevel,
		Reason:               reason,
		Blocked:              action == ActionBlock,
		RequiresConfirmation: action == ActionConfirm,
	}
}
