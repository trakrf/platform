package main

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr bool
	}{
		{"no args -> combined default", []string{}, cmdCombined, false},
		{"serve explicit", []string{"serve"}, cmdServe, false},
		{"migrate explicit", []string{"migrate"}, cmdMigrate, false},
		{"-h prints usage", []string{"-h"}, cmdHelp, false},
		{"--help prints usage", []string{"--help"}, cmdHelp, false},
		{"unknown subcommand is an error", []string{"bogus"}, cmdUnknown, true},
		{"extra args after serve is an error", []string{"serve", "extra"}, cmdUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCommand(%v) err = %v, wantErr = %v", tt.args, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseCommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
