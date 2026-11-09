package serverstatus

import (
	"bytes"
	"errors"
	"io"

	"github.com/UruhaLushia/tomori/locales"
)

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
