package store

import (
	"strings"
	"testing"
)

// hashTag is what Valkey Cluster hashes: the first {...} with something inside, else the key.
func hashTag(key string) string {
	open := strings.IndexByte(key, '{')
	if open < 0 {
		return key
	}
	closing := strings.IndexByte(key[open+1:], '}')
	if closing <= 0 {
		return key
	}
	return key[open+1 : open+1+closing]
}

// Put runs one MULTI over a record and both indexes, and the listings MGET several records.
// A cluster-mode server (ElastiCache Serverless) refuses both unless every key is in one slot.
func TestKeysShareOneHashSlot(t *testing.T) {
	for _, key := range []string{recordPrefix + "mal_1", recordPrefix + "mal_2", activeKey, historyKey} {
		if got := hashTag(key); got != "malaise" {
			t.Errorf("key %q hashes on %q, want the {malaise} tag", key, got)
		}
	}
}
