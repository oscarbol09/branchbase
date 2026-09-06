package pgwire

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func buildMockStartupPacket(user, database string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0}) // length placeholder

	var proto [4]byte
	binary.BigEndian.PutUint32(proto[:], ProtocolVersion3)
	buf.Write(proto[:])

	buf.WriteString("user")
	buf.WriteByte(0)
	buf.WriteString(user)
	buf.WriteByte(0)

	buf.WriteString("database")
	buf.WriteByte(0)
	buf.WriteString(database)
	buf.WriteByte(0)

	buf.WriteByte(0) // terminating null

	data := buf.Bytes()
	binary.BigEndian.PutUint32(data[0:4], uint32(len(data)))
	return data
}

func TestIsSSLRequest(t *testing.T) {
	var sslPkt [8]byte
	binary.BigEndian.PutUint32(sslPkt[0:4], 8)
	binary.BigEndian.PutUint32(sslPkt[4:8], SSLRequestCode)

	if !IsSSLRequest(sslPkt[:]) {
		t.Errorf("expected IsSSLRequest to return true for valid SSL packet")
	}

	nonSSLPkt := []byte{0, 0, 0, 8, 0, 0, 0, 1}
	if IsSSLRequest(nonSSLPkt) {
		t.Errorf("expected IsSSLRequest to return false for non-SSL packet")
	}
}

func TestParseStartupMessage(t *testing.T) {
	pkt := buildMockStartupPacket("postgres", "myapp_dev")

	msg, err := ParseStartupMessage(pkt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Parameters["user"] != "postgres" {
		t.Errorf("expected user 'postgres', got %q", msg.Parameters["user"])
	}
	if msg.Parameters["database"] != "myapp_dev" {
		t.Errorf("expected database 'myapp_dev', got %q", msg.Parameters["database"])
	}
}

func TestRewriteDatabase(t *testing.T) {
	pkt := buildMockStartupPacket("postgres", "myapp_dev")

	rewritten, err := RewriteDatabase(pkt, "myapp_dev_feature_billing")
	if err != nil {
		t.Fatalf("unexpected error rewriting database: %v", err)
	}

	msg, err := ParseStartupMessage(rewritten)
	if err != nil {
		t.Fatalf("unexpected error parsing rewritten packet: %v", err)
	}

	if msg.Parameters["database"] != "myapp_dev_feature_billing" {
		t.Errorf("expected database 'myapp_dev_feature_billing', got %q", msg.Parameters["database"])
	}
	if msg.Parameters["user"] != "postgres" {
		t.Errorf("expected user 'postgres' to remain unchanged, got %q", msg.Parameters["user"])
	}

	// Verify packet length consistency
	expectedLen := binary.BigEndian.Uint32(rewritten[0:4])
	if int(expectedLen) != len(rewritten) {
		t.Errorf("header length %d does not match actual length %d", expectedLen, len(rewritten))
	}
}
