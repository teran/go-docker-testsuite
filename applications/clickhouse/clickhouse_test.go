package clickhouse

import "testing"

func TestValidateDBName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "exceeds 64 bytes", input: string(make([]byte, 65)), wantErr: true},
		{name: "dollar sign", input: "$db", wantErr: true},
		{name: "slash", input: "db/name", wantErr: true},
		{name: "dash rejected", input: "my-db", wantErr: true},
		{name: "space rejected", input: "my db", wantErr: true},
		{name: "ascii valid", input: "ddldb", wantErr: false},
		{name: "ascii valid with underscore", input: "my_db", wantErr: false},
		{name: "uppercase valid", input: "MyDB", wantErr: false},
		{name: "digits valid", input: "db123", wantErr: false},
		{name: "unicode letter rejected", input: "däta", wantErr: true},
		{name: "cyrillic rejected", input: "база", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDBName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateDBName(%q) = nil, want error", tt.input)
				}
			} else {
				if err != nil {
					t.Fatalf("validateDBName(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}
