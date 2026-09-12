//go:build windows

package main

import "testing"

func TestNormalizeScheduledTaskSettingsRestartInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantErr  bool
	}{
		{name: "ten seconds", interval: "PT10S", wantErr: true},
		{name: "one minute", interval: "PT1M", wantErr: false},
		{name: "ninety seconds", interval: "PT90S", wantErr: false},
		{name: "thirty one days", interval: "P31D", wantErr: false},
		{name: "over thirty one days", interval: "P31DT1S", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := defaultScheduledTaskSettings()
			settings.RestartCount = 1
			settings.RestartInterval = test.interval

			_, err := normalizeScheduledTaskSettings(settings)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeScheduledTaskSettings() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestNormalizeScheduledTaskSettingsRestartCountMaximum(t *testing.T) {
	settings := defaultScheduledTaskSettings()
	settings.RestartInterval = "PT1M"
	settings.RestartCount = 256

	if _, err := normalizeScheduledTaskSettings(settings); err == nil {
		t.Fatal("normalizeScheduledTaskSettings() expected an error for restart count 256")
	}
}
