package csmsstomp

import "testing"

func TestParseAPIHost(t *testing.T) {
	host, tls, err := ParseAPIHost("https://cs-test.use-move.com/api/v1")
	if err != nil || host != "cs-test.use-move.com" || !tls {
		t.Fatalf("got host=%q tls=%v err=%v", host, tls, err)
	}
	host2, tls2, err2 := ParseAPIHost("http://localhost:8080/api/v1")
	if err2 != nil || host2 != "localhost:8080" || tls2 {
		t.Fatalf("got host=%q tls=%v err=%v", host2, tls2, err2)
	}
}
