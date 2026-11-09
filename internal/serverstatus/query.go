package serverstatus

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

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
		Players struct {
			Online, Max int
			Sample      []struct{ Name, ID string }
		}
		Description json.RawMessage
		Favicon     string
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Version.Name == "" {
		return status, errors.New(locales.InvalidServerResponse())
	}
	status = Status{Version: clip(data.Version.Name, 256), Protocol: -1, Online: max(0, data.Players.Online), Maximum: max(0, data.Players.Max)}
	for _, player := range data.Players.Sample {
		name := strings.TrimSpace(clip(player.Name, 64))
		if name != "" && len(status.Members) < 512 {
			status.Members = append(status.Members, Member{Name: name, UUID: strings.TrimSpace(player.ID)})
		}
	}
	if _, encoded, ok := strings.Cut(data.Favicon, ","); ok && len(encoded) <= 12<<20 {
		encoded = strings.TrimSpace(encoded)
		icon, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			icon, decodeErr = base64.RawStdEncoding.DecodeString(encoded)
		}
		if decodeErr == nil && len(icon) <= 8<<20 {
			status.Icon = icon
		}
	}
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
