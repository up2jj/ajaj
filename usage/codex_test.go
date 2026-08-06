package usage

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadRPCResponseSkipsNotifications(t *testing.T) {
	input := "{\"method\":\"notice\"}\n{\"id\":2,\"result\":{\"rateLimits\":{}}}\n"
	response, err := readRPCResponse(bufio.NewScanner(strings.NewReader(input)), 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.ID) != "2" {
		t.Fatalf("id = %s", response.ID)
	}
}

func TestWindowName(t *testing.T) {
	for minutes, want := range map[int]string{300: "5h", 10080: "1w", 20160: "2w", 45: "45m"} {
		if got := windowName(minutes); got != want {
			t.Errorf("windowName(%d) = %q; want %q", minutes, got, want)
		}
	}
}
