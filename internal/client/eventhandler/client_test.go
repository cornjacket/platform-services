package eventhandler

import "testing"

func TestTopicFromEventType(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		// Sensor events
		{"sensor.reading", "sensor-events"},
		{"sensor.alert", "sensor-events"},
		{"sensor.calibration", "sensor-events"},

		// User events
		{"user.login", "user-actions"},
		{"user.logout", "user-actions"},
		{"user.signup", "user-actions"},

		// System/default events
		{"system.startup", "system-events"},
		{"unknown.type", "system-events"},
		{"", "system-events"},
		{"no-prefix", "system-events"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			got := topicFromEventType(tt.eventType)
			if got != tt.want {
				t.Errorf("topicFromEventType(%q) = %q, want %q", tt.eventType, got, tt.want)
			}
		})
	}
}
