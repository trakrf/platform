package readstream

import "testing"

// TRA-922: readerKeyFromTopic must extract the middle key for both the new
// {org_slug}/{key}/reads scheme and grandfathered trakrf.id/{key}/reads topics.
func TestReaderKeyFromTopic(t *testing.T) {
	cases := map[string]string{
		"organized-chaos/dock-1/reads": "dock-1", // new slug-as-root scheme
		"trakrf.id/dock-1/reads":       "dock-1", // grandfathered
		"nada/C4DEE229A176/reads":      "C4DEE229A176",
		"not-a-reads-topic":            "not-a-reads-topic", // fallback to whole string
		"a/b/c/reads":                  "a/b/c/reads",       // 4 segments: no match, fallback
	}
	for topic, want := range cases {
		if got := readerKeyFromTopic(topic); got != want {
			t.Errorf("readerKeyFromTopic(%q) = %q, want %q", topic, got, want)
		}
	}
}
