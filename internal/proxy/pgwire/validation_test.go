package pgwire

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDatabaseIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dbName    string
		wantErr   error
		wantMatch bool
	}{
		{
			name:      "valid standard name",
			dbName:    "myapp_dev",
			wantErr:   nil,
			wantMatch: false,
		},
		{
			name:      "valid starting with underscore",
			dbName:    "_private_db",
			wantErr:   nil,
			wantMatch: false,
		},
		{
			name:      "valid single letter",
			dbName:    "a",
			wantErr:   nil,
			wantMatch: false,
		},
		{
			name:      "valid with dollar sign and digits",
			dbName:    "db$123_test",
			wantErr:   nil,
			wantMatch: false,
		},
		{
			name:      "valid 63-byte max length",
			dbName:    strings.Repeat("a", MaxDatabaseIdentifierLen),
			wantErr:   nil,
			wantMatch: false,
		},
		{
			name:      "empty database name",
			dbName:    "",
			wantErr:   ErrEmptyDatabaseName,
			wantMatch: true,
		},
		{
			name:      "database name exceeds 63 bytes",
			dbName:    strings.Repeat("a", MaxDatabaseIdentifierLen+1),
			wantErr:   ErrDatabaseNameTooLong,
			wantMatch: true,
		},
		{
			name:      "database name contains embedded null byte",
			dbName:    "myapp\x00_dev",
			wantErr:   ErrDatabaseNameContainsNull,
			wantMatch: true,
		},
		{
			name:      "database name starts with null byte",
			dbName:    "\x00database",
			wantErr:   ErrDatabaseNameContainsNull,
			wantMatch: true,
		},
		{
			name:      "starts with digit",
			dbName:    "1database",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "contains hyphen",
			dbName:    "myapp-dev",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "contains space",
			dbName:    "my app",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "contains dot",
			dbName:    "public.users",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "contains slash",
			dbName:    "path/to/db",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "SQL injection attempt with semicolon",
			dbName:    "myapp; DROP TABLE users;--",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
		{
			name:      "SQL injection attempt with quotes",
			dbName:    "myapp' OR '1'='1",
			wantErr:   ErrInvalidDatabaseIdentifier,
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateDatabaseIdentifier(tt.dbName)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.dbName, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %v for %q, got nil", tt.wantErr, tt.dbName)
			}
			if tt.wantMatch && !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRewriteDatabaseValidation(t *testing.T) {
	t.Parallel()
	pkt := buildMockStartupPacket("postgres", "myapp_dev")

	invalidNames := []struct {
		name    string
		dbName  string
		wantErr error
	}{
		{"empty", "", ErrEmptyDatabaseName},
		{"too long", strings.Repeat("x", 64), ErrDatabaseNameTooLong},
		{"null byte", "test\x00db", ErrDatabaseNameContainsNull},
		{"invalid char", "test-db", ErrInvalidDatabaseIdentifier},
	}

	for _, tt := range invalidNames {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := RewriteDatabase(pkt, tt.dbName)
			if err == nil {
				t.Fatalf("expected RewriteDatabase to fail for %q, got nil", tt.dbName)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error to wrap %v, got %v", tt.wantErr, err)
			}
		})
	}
}
