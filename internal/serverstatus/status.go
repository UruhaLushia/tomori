package serverstatus

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"

	"github.com/UruhaLushia/tomori/locales"
)

type Address struct {
	Host string
	Port uint16
}

func ParseAddress(value string) (Address, error) {
	value = strings.TrimSpace(value)
	invalid := errors.New(locales.EnterAValidServerAddress())
	if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "/\\@?#\x00") || strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsFunc(value, unicode.IsControl) {
		return Address{}, invalid
	}
	host, port := value, "25565"
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = value[1 : len(value)-1]
	} else if strings.HasPrefix(value, "[") || strings.Count(value, ":") == 1 {
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil {
			return Address{}, invalid
		}
	}
	if strings.HasPrefix(value, "[") {
		if ip, err := netip.ParseAddr(host); err != nil || !ip.Is6() {
			return Address{}, invalid
		}
	}
	if _, err := netip.ParseAddr(host); err != nil {
		if strings.ContainsAny(host, ":[]%") {
			return Address{}, invalid
		}
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || host == "" || host == "." || strings.HasPrefix(host, "-") || strings.Contains(host, "..") {
			return Address{}, invalid
		}
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 || len(host) > 255 {
		return Address{}, invalid
	}
	return Address{Host: host, Port: uint16(number)}, nil
}

func (a Address) String() string { return net.JoinHostPort(a.Host, strconv.Itoa(int(a.Port))) }

type Status struct {
	Version, Description string
	Protocol             int
	Online, Maximum      int
	Latency              time.Duration
}

type Client struct {
	Dial      func(context.Context, string, string) (net.Conn, error)
	LookupSRV func(context.Context, string, string, string) (string, []*net.SRV, error)
}

func (c Client) Query(ctx context.Context, value string, protocol int) (status Status, err error) {
	address, err := ParseAddress(value)
	if err != nil {
		return status, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				err = context.DeadlineExceeded
			}
		}
	}()
	lookup := c.LookupSRV
	if lookup == nil {
		lookup = net.DefaultResolver.LookupSRV
	}
	target := address
	if _, err := netip.ParseAddr(address.Host); err != nil && address.Port == 25565 {
		_, records, err := lookup(ctx, "minecraft", "tcp", address.Host)
		if ctx.Err() != nil {
			return status, ctx.Err()
		}
		if err == nil && len(records) > 0 {
			if records[0].Target == "." || records[0].Port == 0 {
				return status, errors.New(locales.ServerHasNoConnectionAddress())
			}
			target = Address{Host: strings.TrimSuffix(records[0].Target, "."), Port: records[0].Port}
		}
	}
	dial := c.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	}
	conn, err := dial(ctx, "tcp", target.String())
	if err != nil {
		return status, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return status, err
	}
	handshake := appendVarInt([]byte{0}, uint32(protocol))
	handshake = appendVarInt(handshake, uint32(len(address.Host)))
	handshake = append(handshake, address.Host...)
	handshake = binary.BigEndian.AppendUint16(handshake, target.Port)
	handshake = append(handshake, 1)
	if err := writePacket(conn, handshake); err != nil {
		return status, err
	}
	if err := writePacket(conn, []byte{0}); err != nil {
		return status, err
	}
	packet, err := readPacket(conn)
	if err != nil {
		return status, err
	}
	r := bytes.NewReader(packet)
	id, err := readVarInt(r)
	if err != nil || id != 0 {
		return status, errors.New(locales.InvalidServerResponse())
	}
	length, err := readVarInt(r)
	if err != nil || int64(length) != int64(r.Len()) {
		return status, errors.New(locales.InvalidServerResponse())
	}
	raw := packet[len(packet)-r.Len():]
	var data struct {
		Version struct {
			Name     string
			Protocol *int
		}
		Players     struct{ Online, Max int }
		Description json.RawMessage
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Version.Name == "" {
		return status, errors.New(locales.InvalidServerResponse())
	}
	status = Status{Version: clip(data.Version.Name, 256), Protocol: -1, Online: max(0, data.Players.Online), Maximum: max(0, data.Players.Max)}
	if data.Version.Protocol != nil {
		status.Protocol = *data.Version.Protocol
	}
	if len(data.Description) <= 64<<10 {
		var text strings.Builder
		chatText(data.Description, &text, 0)
		status.Description = plainText(text.String())
	}
	start := time.Now()
	nonce := uint64(start.UnixNano())
	if err := writePacket(conn, binary.BigEndian.AppendUint64([]byte{1}, nonce)); err != nil {
		return status, err
	}
	packet, err = readPacket(conn)
	if err != nil {
		return status, err
	}
	if len(packet) != 9 || packet[0] != 1 || binary.BigEndian.Uint64(packet[1:]) != nonce {
		return status, errors.New(locales.InvalidServerResponse())
	}
	status.Latency = time.Since(start)
	return status, nil
}

func appendVarInt(out []byte, value uint32) []byte {
	for value >= 128 {
		out = append(out, byte(value)|128)
		value >>= 7
	}
	return append(out, byte(value))
}

func readVarInt(r io.Reader) (uint32, error) {
	var value uint32
	var b [1]byte
	for i := range 5 {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		if i == 4 && b[0]&0xf0 != 0 {
			return 0, errors.New(locales.InvalidServerResponse())
		}
		value |= uint32(b[0]&127) << uint(i*7)
		if b[0]&128 == 0 {
			return value, nil
		}
	}
	return 0, errors.New(locales.InvalidServerResponse())
}

func writePacket(w io.Writer, packet []byte) error {
	data := appendVarInt(nil, uint32(len(packet)))
	data = append(data, packet...)
	_, err := io.Copy(w, bytes.NewReader(data))
	return err
}

func readPacket(r io.Reader) ([]byte, error) {
	n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if n == 0 || n > 1<<20 {
		return nil, errors.New(locales.ServerResponseExceedsTheSizeLimit())
	}
	packet := make([]byte, n)
	_, err = io.ReadFull(r, packet)
	return packet, err
}

func chatText(raw []byte, out *strings.Builder, depth int) {
	if depth > 16 || out.Len() >= 4096 || len(raw) == 0 {
		return
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			out.WriteString(clip(text, 4096-out.Len()))
		}
		return
	}
	if raw[0] == '[' {
		var parts []json.RawMessage
		if json.Unmarshal(raw, &parts) == nil {
			for _, part := range parts {
				chatText(part, out, depth+1)
				if out.Len() >= 4096 {
					break
				}
			}
		}
		return
	}
	var part struct {
		Text, Fallback, Translate string
		Extra                     []json.RawMessage
	}
	if json.Unmarshal(raw, &part) != nil {
		return
	}
	text := part.Text
	if text == "" {
		text = part.Fallback
	}
	if text == "" {
		text = part.Translate
	}
	out.WriteString(clip(text, 4096-out.Len()))
	for _, child := range part.Extra {
		chatText(child, out, depth+1)
		if out.Len() >= 4096 {
			break
		}
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func plainText(s string) string {
	var b strings.Builder
	format := false
	for _, r := range s {
		if format {
			format = false
			continue
		}
		if r == '§' {
			format = true
			continue
		}
		if !unicode.IsControl(r) || r == '\n' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func (s Status) Compatible(protocol int) bool {
	return protocol < 0 || s.Protocol < 0 || protocol == s.Protocol
}

func (s Status) Players() string { return fmt.Sprintf("%d / %d", s.Online, s.Maximum) }
