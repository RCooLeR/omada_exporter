package model

import (
	"encoding/json"
	"testing"
)

func TestStringListUnmarshalFlexibleShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "string",
			body: `{"values":"10.0.0.0/24, 192.168.1.0/24"}`,
			want: "10.0.0.0/24,192.168.1.0/24",
		},
		{
			name: "array",
			body: `{"values":["192.168.1.0/24","10.0.0.0/24"]}`,
			want: "10.0.0.0/24,192.168.1.0/24",
		},
		{
			name: "object array",
			body: `{"values":[{"cidr":"192.168.1.0/24"},{"ip":"10.0.0.2"}]}`,
			want: "10.0.0.2,192.168.1.0/24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded struct {
				Values StringList `json:"values"`
			}
			if err := json.Unmarshal([]byte(tt.body), &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if got := decoded.Values.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
