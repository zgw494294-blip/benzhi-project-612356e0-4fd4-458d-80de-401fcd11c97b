package main

import "testing"

func TestAddressPriority(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		explicit  bool
		port      string
		want      string
		wantError bool
	}{
		{name: "default", want: "127.0.0.1:19081"},
		{name: "port", port: "19123", want: "127.0.0.1:19123"},
		{name: "flag wins", flag: "127.0.0.1:19234", explicit: true, port: "19123", want: "127.0.0.1:19234"},
		{name: "empty explicit", explicit: true, wantError: true},
		{name: "external", flag: "0.0.0.0:19081", explicit: true, wantError: true},
		{name: "bad port env", port: "8080x", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveAddress(test.flag, test.explicit, test.port)
			if test.wantError && err == nil {
				t.Fatalf("expected error, got %s", got)
			}
			if !test.wantError && (err != nil || got != test.want) {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
