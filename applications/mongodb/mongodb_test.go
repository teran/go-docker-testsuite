package mongodb

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
		{name: "leading dot", input: ".db", wantErr: true},
		{name: "dot anywhere", input: "db.name", wantErr: true},
		{name: "ascii valid", input: "ddldb", wantErr: false},
		{name: "unicode valid", input: "dätaбаза", wantErr: false},
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
