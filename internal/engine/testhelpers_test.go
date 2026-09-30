package engine

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func hostnameT(t *testing.T) string {
	h, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func itoaT(i int) string              { return strconv.Itoa(i) }
func writeFileT(p, body string) error { return os.WriteFile(p, []byte(body), 0o600) }
func chtimesT(p string, tm time.Time) { os.Chtimes(p, tm, tm) }
