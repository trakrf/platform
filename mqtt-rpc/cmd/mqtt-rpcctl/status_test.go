package main

import "testing"

func TestSummarizeStatus(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"healthy", `{"online":true,"version":"v1","health":{"api":"healthy","wedged":false,"auto_reboots":0,"auto_reboots_exhausted":false,"golden_config":"applied"}}`,
			"online v1 api=healthy golden=applied"},
		{"wedged mid-recovery", `{"online":true,"version":"v1","health":{"api":"degraded","detail":"http 200 with an empty body","wedged":true,"recovery":"app_restart","auto_reboots":1,"auto_reboots_exhausted":false,"golden_config":"pending"}}`,
			`online v1 api=degraded WEDGED recovery=app_restart auto_reboots=1 golden=pending detail="http 200 with an empty body"`},
		{"exhausted", `{"online":true,"version":"v1","health":{"api":"degraded","wedged":true,"recovery":"reboot","auto_reboots":2,"auto_reboots_exhausted":true,"golden_config":"pending"}}`,
			"online v1 api=degraded WEDGED recovery=reboot auto_reboots=2 EXHAUSTED golden=pending"},
		{"cloudserver fault on a healthy reader", `{"online":true,"version":"v1","health":{"api":"healthy","wedged":false,"auto_reboots":0,"auto_reboots_exhausted":false,"golden_config":"applied","cloudserver_fault":"cleanSession is false; golden is true"}}`,
			`online v1 api=healthy golden=applied cloudserver_fault="cleanSession is false; golden is true"`},
		{"planned offline", `{"online":false,"reason":"auto-recovery","version":"v1"}`,
			"offline (auto-recovery) v1"},
		{"last will", `{"online":false}`, "offline"},
		{"older daemon without health", `{"online":true,"version":"v0"}`, "online v0"},
		{"not json", `garbage`, "unparseable: garbage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := summarizeStatus([]byte(c.in)); got != c.want {
				t.Errorf("summarizeStatus =\n  %s\nwant\n  %s", got, c.want)
			}
		})
	}
}
