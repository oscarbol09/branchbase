package risk

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// HeuristicClassifier evaluates migration risk using deterministic statement rules and catalog metadata.
type HeuristicClassifier struct{}

// NewHeuristicClassifier returns an instance of HeuristicClassifier.
func NewHeuristicClassifier() *HeuristicClassifier {
	return &HeuristicClassifier{}
}

func (h *HeuristicClassifier) Name() string {
	return "heuristic"
}

var (
	reCascade          = regexp.MustCompile(`(?i)\bCASCADE\b`)
	reDataManipulation = regexp.MustCompile(`(?i)^\s*(UPDATE|DELETE)\s+`)
	reNotNull          = regexp.MustCompile(`(?i)\bNOT\s+NULL\b`)
	reWordDefault      = regexp.MustCompile(`(?i)\bDEFAULT\b`)
	reWordWhere        = regexp.MustCompile(`(?i)\bWHERE\b`)
)

// Classify evaluates the migration operations against deterministic risk heuristics.
func (h *HeuristicClassifier) Classify(ctx context.Context, input ClassificationInput) (*MigrationRisk, error) {
	level := RiskLow
	category := CategorySchemaChange
	reversible := true
	destructive := false
	touchesFKs := false
	exclusiveLock := false
	var flags []string
	var suggestions []string
	var details []string

	confidence := 0.95

	// If no operations extracted, classify as docs or empty
	if len(input.Operations) == 0 {
		cleaned := StripCommentsAndTransactions(input.SQL)
		if cleaned == "" {
			return &MigrationRisk{
				File:        input.File,
				Level:       RiskLow,
				Reversible:  true,
				Destructive: false,
				Category:    CategoryDocsOrComments,
				Confidence:  1.0,
				Flags:       []string{"EMPTY_OR_COMMENTS_ONLY"},
				AnalyzedAt:  time.Now(),
				Engine:      input.Engine,
				Source:      h.Name(),
			}, nil
		}
	}

	for _, op := range input.Operations {
		upperSQL := strings.ToUpper(op.RawSQL)

		switch op.Type {
		case "DROP_TABLE":
			level = maxLevel(level, RiskCritical)
			destructive = true
			reversible = false
			exclusiveLock = true
			flags = append(flags, fmt.Sprintf("DROP TABLE %s", op.TargetTable))
			details = append(details, fmt.Sprintf("Table %q and all contained rows will be permanently destroyed.", op.TargetTable))
			suggestions = append(suggestions, fmt.Sprintf("Archive table %q data before dropping or use a soft-delete deprecation strategy.", op.TargetTable))

		case "TRUNCATE":
			level = maxLevel(level, RiskCritical)
			destructive = true
			reversible = false
			exclusiveLock = true
			category = CategoryDataMigration
			flags = append(flags, fmt.Sprintf("TRUNCATE %s", op.TargetTable))
			details = append(details, fmt.Sprintf("All data in table %q will be removed without individual row logging.", op.TargetTable))

		case "DROP_COLUMN":
			destructive = true
			reversible = false
			exclusiveLock = true
			colName := op.TargetColumn

			// Check foreign key dependencies
			isFKReferenced := false
			targetMeta := input.TableMetadata
			if input.TablesMetadata != nil && input.TablesMetadata[op.TargetTable] != nil {
				targetMeta = input.TablesMetadata[op.TargetTable]
			}
			if targetMeta != nil {
				for _, fk := range targetMeta.InboundFKs {
					if strings.EqualFold(fk.ToColumn, colName) || colName == "" {
						isFKReferenced = true
						touchesFKs = true
						flags = append(flags, fmt.Sprintf("DROP COLUMN %s referenced by FK in %s(%s)", colName, fk.FromTable, fk.FromColumn))
						details = append(details, fmt.Sprintf("Column %q in table %q is referenced by FK constraint %q in table %q.",
							colName, op.TargetTable, fk.ConstraintName, fk.FromTable))
					}
				}
			}

			if isFKReferenced {
				level = maxLevel(level, RiskCritical)
				suggestions = append(suggestions, fmt.Sprintf("Remove or reassign dependent foreign keys from %s before dropping column %q.", op.TargetTable, colName))
			} else {
				level = maxLevel(level, RiskHigh)
				flags = append(flags, fmt.Sprintf("DROP COLUMN %s.%s", op.TargetTable, colName))
				if targetMeta == nil || len(targetMeta.Columns) == 0 {
					flags = append(flags, "FK impact unknown: catalog metadata unavailable")
					details = append(details, "The database catalog could not confirm whether foreign keys reference this column.")
				}
				suggestions = append(suggestions, fmt.Sprintf("Ensure column %q has been decommissioned from application code in prior release before dropping.", colName))
			}

		case "ALTER_TYPE":
			level = maxLevel(level, RiskHigh)
			exclusiveLock = true
			flags = append(flags, fmt.Sprintf("ALTER COLUMN TYPE %s.%s", op.TargetTable, op.TargetColumn))
			details = append(details, fmt.Sprintf("Changing column type for %s.%s can cause data truncation, type cast failures, or full table rewrite.", op.TargetTable, op.TargetColumn))
			suggestions = append(suggestions, "Add a new column with target type, dual-write in application, backfill, and drop old column.")

		case "DROP_CONSTRAINT":
			level = maxLevel(level, RiskHigh)
			category = CategoryConstraint
			exclusiveLock = true
			flags = append(flags, fmt.Sprintf("DROP CONSTRAINT on %s", op.TargetTable))
			details = append(details, fmt.Sprintf("Dropping constraint on %s removes relational integrity guarantees.", op.TargetTable))

		case "DROP_INDEX":
			level = maxLevel(level, RiskMedium)
			category = CategoryIndex
			flags = append(flags, fmt.Sprintf("DROP INDEX %s", op.TargetTable))
			details = append(details, "Dropping index may degrade query performance.")

		case "ADD_CONSTRAINT":
			category = CategoryConstraint
			if strings.EqualFold(input.Engine, "postgres") && !strings.Contains(upperSQL, "NOT VALID") {
				level = maxLevel(level, RiskMedium)
				exclusiveLock = true
				flags = append(flags, "ADD CONSTRAINT without NOT VALID")
				suggestions = append(suggestions, "Use ADD CONSTRAINT ... NOT VALID followed by VALIDATE CONSTRAINT to avoid table locking.")
			} else {
				level = maxLevel(level, RiskLow)
			}

		case "ADD_INDEX":
			category = CategoryIndex
			if strings.EqualFold(input.Engine, "postgres") && !strings.Contains(upperSQL, "CONCURRENTLY") {
				level = maxLevel(level, RiskMedium)
				exclusiveLock = true
				flags = append(flags, "CREATE INDEX without CONCURRENTLY")
				suggestions = append(suggestions, "Use CREATE INDEX CONCURRENTLY in PostgreSQL to prevent locking concurrent writes.")
			} else {
				level = maxLevel(level, RiskLow)
			}

		case "ADD_COLUMN":
			if reNotNull.MatchString(op.RawSQL) && !reWordDefault.MatchString(op.RawSQL) {
				level = maxLevel(level, RiskHigh)
				exclusiveLock = true
				flags = append(flags, "ADD COLUMN NOT NULL without DEFAULT")
				details = append(details, "Adding NOT NULL column without DEFAULT will fail if table already contains rows.")
				suggestions = append(suggestions, "Add column as NULLABLE, backfill existing records, then add NOT NULL constraint.")
			} else {
				level = maxLevel(level, RiskLow)
			}

		case "CREATE_TABLE":
			level = maxLevel(level, RiskLow)
			reversible = true

		case "GENERIC_DDL":
			if reDataManipulation.MatchString(op.RawSQL) {
				category = CategoryDataMigration
				if !reWordWhere.MatchString(op.RawSQL) {
					level = maxLevel(level, RiskHigh)
					destructive = true
					flags = append(flags, "DML UPDATE/DELETE without WHERE clause")
					details = append(details, "Data modification without WHERE clause modifies all rows in table.")
				} else {
					level = maxLevel(level, RiskMedium)
				}
			} else {
				level = maxLevel(level, RiskHigh)
				flags = append(flags, "UNCLASSIFIED SQL statement")
				details = append(details, "This SQL statement is outside the supported migration parser; review it manually before applying.")
			}

		case "RENAME_COLUMN", "RENAME_TABLE":
			level = maxLevel(level, RiskMedium)
			exclusiveLock = true
			flags = append(flags, fmt.Sprintf("RENAME on %s", op.TargetTable))
			details = append(details, "Renaming table or column will break queries from running application versions unless staged.")

		default:
			// Check for data migration DML
			if reDataManipulation.MatchString(op.RawSQL) {
				category = CategoryDataMigration
				if !reWordWhere.MatchString(op.RawSQL) {
					level = maxLevel(level, RiskHigh)
					destructive = true
					flags = append(flags, "DML UPDATE/DELETE without WHERE clause")
					details = append(details, "Data modification without WHERE clause modifies all rows in table.")
				} else {
					level = maxLevel(level, RiskMedium)
				}
			}
		}

		if reCascade.MatchString(op.RawSQL) {
			flags = append(flags, "CASCADE option detected")
			level = maxLevel(level, RiskHigh)
		}
	}

	// Check if any inbound foreign key was touched across all operations
	for _, op := range input.Operations {
		if op.Type == "DROP_TABLE" {
			targetMeta := input.TableMetadata
			if input.TablesMetadata != nil && input.TablesMetadata[op.TargetTable] != nil {
				targetMeta = input.TablesMetadata[op.TargetTable]
			}
			if targetMeta != nil && len(targetMeta.InboundFKs) > 0 {
				touchesFKs = true
				level = maxLevel(level, RiskCritical)
				break
			}
		}
	}

	suggestionStr := ""
	if len(suggestions) > 0 {
		suggestionStr = strings.Join(uniqueStrings(suggestions), " | ")
	}

	detailStr := ""
	if len(details) > 0 {
		detailStr = strings.Join(uniqueStrings(details), " ")
	}

	return &MigrationRisk{
		File:          input.File,
		Level:         level,
		Reversible:    reversible,
		Destructive:   destructive,
		TouchesFKs:    touchesFKs,
		ExclusiveLock: exclusiveLock,
		Category:      category,
		Confidence:    confidence,
		Flags:         uniqueStrings(flags),
		Suggestion:    suggestionStr,
		Details:       detailStr,
		Operations:    input.Operations,
		AnalyzedAt:    time.Now(),
		Engine:        input.Engine,
		Source:        h.Name(),
	}, nil
}

func maxLevel(a, b RiskLevel) RiskLevel {
	if a.Order() >= b.Order() {
		return a
	}
	return b
}

func uniqueStrings(list []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range list {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
