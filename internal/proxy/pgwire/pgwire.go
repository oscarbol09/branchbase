package pgwire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	// SSLRequestCode is the special protocol version indicating an SSL handshake request
	SSLRequestCode = 80877103 // 1234.5679 in hex (0x04D2162F)
	// ProtocolVersion3 is the PostgreSQL 3.0 protocol version number
	ProtocolVersion3 = 196608 // 3.0 in hex (0x00030000)
	// MaxDatabaseIdentifierLen is the maximum identifier length in PostgreSQL (NAMEDATALEN - 1)
	MaxDatabaseIdentifierLen = 63
)

var (
	ErrPacketTooShort = errors.New("pgwire: packet length is too short")
	ErrPacketTooLarge = errors.New("pgwire: startup packet exceeds maximum allowed size (10KB)")
	ErrInvalidPacket  = errors.New("pgwire: malformed startup packet")
)

var (
	ErrEmptyDatabaseName         = errors.New("pgwire: database name cannot be empty")
	ErrDatabaseNameTooLong       = errors.New("pgwire: database name exceeds PostgreSQL 63-byte limit")
	ErrDatabaseNameContainsNull  = errors.New("pgwire: database name cannot contain null bytes")
	ErrInvalidDatabaseIdentifier = errors.New("pgwire: invalid characters in database name")
)

// validIdentifierRe matches standard PostgreSQL unquoted identifier syntax:
// must start with an ASCII letter or underscore, followed by alphanumeric characters, underscores, or dollar signs.
var validIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_$]*$`)

// ValidateDatabaseIdentifier ensures a database name adheres to PostgreSQL wire and naming rules:
// - Must not be empty
// - Must not exceed 63 bytes (NAMEDATALEN - 1)
// - Must not contain null bytes (\x00) which would truncate or corrupt packet framing
// - Must match standard PostgreSQL unquoted identifier pattern (^[a-zA-Z_][a-zA-Z0-9_$]*$)
func ValidateDatabaseIdentifier(name string) error {
	if name == "" {
		return ErrEmptyDatabaseName
	}
	if len(name) > MaxDatabaseIdentifierLen {
		return fmt.Errorf("%w: %d bytes (maximum allowed is %d)", ErrDatabaseNameTooLong, len(name), MaxDatabaseIdentifierLen)
	}
	if strings.ContainsRune(name, '\x00') {
		return ErrDatabaseNameContainsNull
	}
	if !validIdentifierRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidDatabaseIdentifier, name)
	}
	return nil
}

// StartupMessage represents a parsed PostgreSQL startup message
type StartupMessage struct {
	ProtocolVersion int32
	Parameters      map[string]string
}

// IsSSLRequest checks whether the initial 8 bytes represent an SSL negotiation request
func IsSSLRequest(header []byte) bool {
	if len(header) < 8 {
		return false
	}
	length := binary.BigEndian.Uint32(header[0:4])
	code := binary.BigEndian.Uint32(header[4:8])
	return length == 8 && code == SSLRequestCode
}

// ReadStartupPacket reads and returns the raw startup packet from the reader
func ReadStartupPacket(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("failed to read packet length: %w", err)
	}

	pktLen := binary.BigEndian.Uint32(lenBuf[:])
	if pktLen < 8 {
		return nil, ErrPacketTooShort
	}
	if pktLen > 10240 { // Sanity check: max 10KB startup packet
		return nil, ErrPacketTooLarge
	}

	payload := make([]byte, pktLen)
	copy(payload[0:4], lenBuf[:])

	if _, err := io.ReadFull(r, payload[4:]); err != nil {
		return nil, fmt.Errorf("failed to read startup payload: %w", err)
	}

	return payload, nil
}

// ParseStartupMessage decodes the key-value parameters from a PostgreSQL StartupMessage packet
func ParseStartupMessage(packet []byte) (*StartupMessage, error) {
	if len(packet) < 8 {
		return nil, ErrPacketTooShort
	}

	proto := int32(binary.BigEndian.Uint32(packet[4:8]))
	params := make(map[string]string)

	// Parameters start at byte 8 and end with a terminating '\0'
	data := packet[8:]
	// Strip the final trailing null byte if present
	if len(data) > 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}

	parts := bytes.Split(data, []byte{0})
	for i := 0; i+1 < len(parts); i += 2 {
		key := string(parts[i])
		if key == "" {
			break
		}
		val := string(parts[i+1])
		params[key] = val
	}

	return &StartupMessage{
		ProtocolVersion: proto,
		Parameters:      params,
	}, nil
}

// RewriteDatabase modifies the 'database' parameter in the packet and re-encodes it with correct length
func RewriteDatabase(packet []byte, newDatabase string) ([]byte, error) {
	if err := ValidateDatabaseIdentifier(newDatabase); err != nil {
		return nil, fmt.Errorf("invalid target database: %w", err)
	}

	msg, err := ParseStartupMessage(packet)
	if err != nil {
		return nil, err
	}

	msg.Parameters["database"] = newDatabase

	var buf bytes.Buffer
	// Placeholder for 4-byte packet length
	buf.Write([]byte{0, 0, 0, 0})

	// Protocol version
	var protoBuf [4]byte
	binary.BigEndian.PutUint32(protoBuf[:], uint32(msg.ProtocolVersion))
	buf.Write(protoBuf[:])

	// Parameters as null-terminated key-value pairs
	for k, v := range msg.Parameters {
		buf.WriteString(k)
		buf.WriteByte(0)
		buf.WriteString(v)
		buf.WriteByte(0)
	}
	// Final terminating null byte
	buf.WriteByte(0)

	result := buf.Bytes()
	binary.BigEndian.PutUint32(result[0:4], uint32(len(result)))

	return result, nil
}

// BuildErrorResponse constructs a PostgreSQL wire protocol 'E' (ErrorResponse) packet.
// The packet contains Severity ('S'), SQLSTATE code ('C'), and Message ('M') fields.
func BuildErrorResponse(severity, code, message string) []byte {
	if severity == "" {
		severity = "FATAL"
	}
	if code == "" {
		code = "XX000" // internal_error
	}

	var buf bytes.Buffer
	// Message type byte 'E'
	buf.WriteByte('E')

	// Length placeholder (4 bytes)
	buf.Write([]byte{0, 0, 0, 0})

	// Severity ('S')
	buf.WriteByte('S')
	buf.WriteString(severity)
	buf.WriteByte(0)

	// SQLSTATE Code ('C')
	buf.WriteByte('C')
	buf.WriteString(code)
	buf.WriteByte(0)

	// Primary Message ('M')
	buf.WriteByte('M')
	buf.WriteString(message)
	buf.WriteByte(0)

	// Terminating zero byte
	buf.WriteByte(0)

	result := buf.Bytes()
	// Length excludes the leading 'E' type byte
	binary.BigEndian.PutUint32(result[1:5], uint32(len(result)-1))

	return result
}


