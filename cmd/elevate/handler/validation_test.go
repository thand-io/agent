package handler

import "testing"

func TestValidateRequestUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{name: "empty", username: "", wantErr: true},
		{name: "starts with letter", username: "alice", wantErr: false},
		{name: "starts with underscore", username: "_svc", wantErr: false},
		{name: "allows dot and hyphen", username: "alice.dev-1", wantErr: false},
		{name: "allows underscore and trailing dollar", username: "svc_app$", wantErr: false},
		{name: "rejects leading hyphen", username: "-alice", wantErr: true},
		{name: "rejects leading dot", username: ".alice", wantErr: true},
		{name: "rejects slash", username: "/delete", wantErr: true},
		{name: "rejects whitespace", username: "alice bob", wantErr: true},
		{name: "rejects trailing whitespace", username: "alice ", wantErr: true},
		{name: "rejects overlong", username: "abcdefghijklmnopqrstuvwxyzabcdefg", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequestUsername(tt.username)
			if tt.wantErr && err == nil {
				t.Fatalf("expected validation error for %q", tt.username)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error for %q: %v", tt.username, err)
			}
		})
	}
}
