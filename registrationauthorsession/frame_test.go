package registrationauthorsession

import (
	"bytes"
	"strings"
	"testing"
)

func TestOutputFrameBoundPrecedesWrite(t *testing.T) {
	var output bytes.Buffer
	server := server{protocol: ProtocolV3, output: &output}
	if server.write(ServerMessage{Type: "hello", Capabilities: []string{strings.Repeat("x", MaxProtocolLineBytes)}}) == nil || output.Len() != 0 {
		t.Fatal("oversized frame reached output")
	}
}
